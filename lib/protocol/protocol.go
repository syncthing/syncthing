// Copyright (C) 2014 The Protocol Authors.

//go:generate -command counterfeiter go run github.com/maxbrunsfeld/counterfeiter/v6

// Prevents import loop, for internal testing
//go:generate counterfeiter -o mocked_connection_info_test.go --fake-name mockedConnectionInfo . ConnectionInfo
//go:generate go run ../../script/prune_mocks.go -t mocked_connection_info_test.go

//go:generate counterfeiter -o mocks/connection_info.go --fake-name ConnectionInfo . ConnectionInfo
//go:generate counterfeiter -o mocks/connection.go --fake-name Connection . Connection

package protocol

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"strings"
	"sync"
	"time"

	lz4 "github.com/pierrec/lz4/v4"
)

const (
	// Shifts
	KiB = 10
	MiB = 20
	GiB = 30
)

const (
	// MaxMessageLen is the largest message size allowed on the wire. (500 MB)
	MaxMessageLen = 500 * 1000 * 1000

	// MinBlockSize is the minimum block size allowed
	MinBlockSize = 128 << KiB

	// MaxBlockSize is the maximum block size allowed
	MaxBlockSize = 16 << MiB

	// DesiredPerFileBlocks is the number of blocks we aim for per file
	DesiredPerFileBlocks = 2000
)

// BlockSizes is the list of valid block sizes, from min to max
var BlockSizes []int

// For each block size, the hash of a block of all zeroes
var sha256OfEmptyBlock = map[int][sha256.Size]byte{
	128 << KiB: {0xfa, 0x43, 0x23, 0x9b, 0xce, 0xe7, 0xb9, 0x7c, 0xa6, 0x2f, 0x0, 0x7c, 0xc6, 0x84, 0x87, 0x56, 0xa, 0x39, 0xe1, 0x9f, 0x74, 0xf3, 0xdd, 0xe7, 0x48, 0x6d, 0xb3, 0xf9, 0x8d, 0xf8, 0xe4, 0x71},
	256 << KiB: {0x8a, 0x39, 0xd2, 0xab, 0xd3, 0x99, 0x9a, 0xb7, 0x3c, 0x34, 0xdb, 0x24, 0x76, 0x84, 0x9c, 0xdd, 0xf3, 0x3, 0xce, 0x38, 0x9b, 0x35, 0x82, 0x68, 0x50, 0xf9, 0xa7, 0x0, 0x58, 0x9b, 0x4a, 0x90},
	512 << KiB: {0x7, 0x85, 0x4d, 0x2f, 0xef, 0x29, 0x7a, 0x6, 0xba, 0x81, 0x68, 0x5e, 0x66, 0xc, 0x33, 0x2d, 0xe3, 0x6d, 0x5d, 0x18, 0xd5, 0x46, 0x92, 0x7d, 0x30, 0xda, 0xad, 0x6d, 0x7f, 0xda, 0x15, 0x41},
	1 << MiB:   {0x30, 0xe1, 0x49, 0x55, 0xeb, 0xf1, 0x35, 0x22, 0x66, 0xdc, 0x2f, 0xf8, 0x6, 0x7e, 0x68, 0x10, 0x46, 0x7, 0xe7, 0x50, 0xab, 0xb9, 0xd3, 0xb3, 0x65, 0x82, 0xb8, 0xaf, 0x90, 0x9f, 0xcb, 0x58},
	2 << MiB:   {0x56, 0x47, 0xf0, 0x5e, 0xc1, 0x89, 0x58, 0x94, 0x7d, 0x32, 0x87, 0x4e, 0xeb, 0x78, 0x8f, 0xa3, 0x96, 0xa0, 0x5d, 0xb, 0xab, 0x7c, 0x1b, 0x71, 0xf1, 0x12, 0xce, 0xb7, 0xe9, 0xb3, 0x1e, 0xee},
	4 << MiB:   {0xbb, 0x9f, 0x8d, 0xf6, 0x14, 0x74, 0xd2, 0x5e, 0x71, 0xfa, 0x0, 0x72, 0x23, 0x18, 0xcd, 0x38, 0x73, 0x96, 0xca, 0x17, 0x36, 0x60, 0x5e, 0x12, 0x48, 0x82, 0x1c, 0xc0, 0xde, 0x3d, 0x3a, 0xf8},
	8 << MiB:   {0x2d, 0xae, 0xb1, 0xf3, 0x60, 0x95, 0xb4, 0x4b, 0x31, 0x84, 0x10, 0xb3, 0xf4, 0xe8, 0xb5, 0xd9, 0x89, 0xdc, 0xc7, 0xbb, 0x2, 0x3d, 0x14, 0x26, 0xc4, 0x92, 0xda, 0xb0, 0xa3, 0x5, 0x3e, 0x74},
	16 << MiB:  {0x8, 0xa, 0xcf, 0x35, 0xa5, 0x7, 0xac, 0x98, 0x49, 0xcf, 0xcb, 0xa4, 0x7d, 0xc2, 0xad, 0x83, 0xe0, 0x1b, 0x75, 0x66, 0x3a, 0x51, 0x62, 0x79, 0xc8, 0xb9, 0xd2, 0x43, 0xb7, 0x19, 0x64, 0x3e},
}

var errNotCompressible = errors.New("not compressible")

func init() {
	for blockSize := MinBlockSize; blockSize <= MaxBlockSize; blockSize *= 2 {
		BlockSizes = append(BlockSizes, blockSize)
		if _, ok := sha256OfEmptyBlock[blockSize]; !ok {
			panic("missing hard coded value for sha256 of empty block")
		}
	}
	BufferPool = newBufferPool()
}

// BlockSize returns the block size to use for the given file size
func BlockSize(fileSize int64) int {
	var blockSize int
	for _, blockSize = range BlockSizes {
		if fileSize < DesiredPerFileBlocks*int64(blockSize) {
			break
		}
	}

	return blockSize
}

const (
	stateInitial = iota
	stateReady
)

// FileInfo.LocalFlags flags
const (
	FlagLocalUnsupported = 1 << 0 // The kind is unsupported, e.g. symlinks on Windows
	FlagLocalIgnored     = 1 << 1 // Matches local ignore patterns
	FlagLocalMustRescan  = 1 << 2 // Doesn't match content on disk, must be rechecked fully
	FlagLocalReceiveOnly = 1 << 3 // Change detected on receive only folder

	// Flags that should result in the Invalid bit on outgoing updates
	LocalInvalidFlags = FlagLocalUnsupported | FlagLocalIgnored | FlagLocalMustRescan | FlagLocalReceiveOnly

	// Flags that should result in a file being in conflict with its
	// successor, due to us not having an up to date picture of its state on
	// disk.
	LocalConflictFlags = FlagLocalUnsupported | FlagLocalIgnored | FlagLocalReceiveOnly

	LocalAllFlags = FlagLocalUnsupported | FlagLocalIgnored | FlagLocalMustRescan | FlagLocalReceiveOnly
)

var (
	ErrClosed             = errors.New("connection closed")
	ErrTimeout            = errors.New("read timeout")
	errUnknownMessage     = errors.New("unknown message")
	errInvalidFilename    = errors.New("filename is invalid")
	errUncleanFilename    = errors.New("filename not in canonical format")
	errDeletedHasBlocks   = errors.New("deleted file with non-empty block list")
	errDirectoryHasBlocks = errors.New("directory with non-empty block list")
	errFileHasNoBlocks    = errors.New("file with empty block list")
)

type contextLessModel interface {
	// An index was received from the peer device
	Index(folder string, files []FileInfo) error
	// An index update was received from the peer device
	IndexUpdate(folder string, files []FileInfo) error
	// A request was made by the peer device
	Request(folder, name string, blockNo, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (RequestResponse, error)
	// A cluster configuration message was received
	ClusterConfig(config ClusterConfig) error
	// The peer device closed the connection or an error occurred
	Closed(err error)
	// The peer device sent progress updates for the files it is currently downloading
	DownloadProgress(folder string, updates []FileDownloadProgressUpdate) error
}

type Model interface {
	// An index was received from the peer device
	Index(conn Connection, folder string, files []FileInfo) error
	// An index update was received from the peer device
	IndexUpdate(conn Connection, folder string, files []FileInfo) error
	// A request was made by the peer device
	Request(conn Connection, folder, name string, blockNo, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (RequestResponse, error)
	// A cluster configuration message was received
	ClusterConfig(conn Connection, config ClusterConfig) error
	// The peer device closed the connection or an error occurred
	Closed(conn Connection, err error)
	// The peer device sent progress updates for the files it is currently downloading
	DownloadProgress(conn Connection, folder string, updates []FileDownloadProgressUpdate) error
}

type RequestResponse interface {
	Data() []byte
	Close() // Must always be called once the byte slice is no longer in use
	Wait()  // Blocks until Close is called
}

type Connection interface {
	Start()
	SetFolderPasswords(passwords map[string]string)
	Close(err error)
	DeviceID() DeviceID
	Index(ctx context.Context, folder string, files []FileInfo) error
	IndexUpdate(ctx context.Context, folder string, files []FileInfo) error
	Request(ctx context.Context, folder string, name string, blockNo int, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error)
	ClusterConfig(config ClusterConfig)
	DownloadProgress(ctx context.Context, folder string, updates []FileDownloadProgressUpdate)
	Statistics() Statistics
	Closed() <-chan struct{}
	ConnectionInfo
}

type ConnectionInfo interface {
	Type() string
	Transport() string
	IsLocal() bool
	RemoteAddr() net.Addr
	Priority() int
	String() string
	Crypto() string
	EstablishedAt() time.Time
}

type rawConnection struct {
	ConnectionInfo

	deviceID  DeviceID
	model     contextLessModel
	startTime time.Time

	cr     *countingReader
	cw     *countingWriter
	closer io.Closer // Closing the underlying connection and thus cr and cw

	awaitingMut sync.Mutex // Protects awaiting and nextID.
	awaiting    map[int]chan asyncResult
	nextID      int

	idxMut sync.Mutex // ensures serialization of Index calls

	inbox                 chan message
	outbox                chan asyncMessage
	closeBox              chan asyncMessage
	clusterConfigBox      chan *ClusterConfig
	dispatcherLoopStopped chan struct{}
	closed                chan struct{}
	closeOnce             sync.Once
	sendCloseOnce         sync.Once
	compression           Compression

	loopWG sync.WaitGroup // Need to ensure no leftover routines in testing
}

type asyncResult struct {
	val []byte
	err error
}

type message interface {
	ProtoSize() int
	Marshal() ([]byte, error)
	MarshalTo([]byte) (int, error)
	Unmarshal([]byte) error
}

type asyncMessage struct {
	msg  message
	done chan struct{} // done closes when we're done sending the message
}

const (
	// PingSendInterval is how often we make sure to send a message, by
	// triggering pings if necessary.
	PingSendInterval = 90 * time.Second
	// ReceiveTimeout is the longest we'll wait for a message from the other
	// side before closing the connection.
	ReceiveTimeout = 300 * time.Second
)

// CloseTimeout is the longest we'll wait when trying to send the close
// message before just closing the connection.
// Should not be modified in production code, just for testing.
var CloseTimeout = 10 * time.Second

func NewConnection(deviceID DeviceID, reader io.Reader, writer io.Writer, closer io.Closer, model Model, connInfo ConnectionInfo, compress Compression, passwords map[string]string, keyGen *KeyGenerator) Connection {
	// We create the wrapper for the model first, as it needs to be passed
	// in at the lowest level in the stack. At the end of construction,
	// before returning, we add the connection to cwm so that it can be used
	// by the model.
	cwm := &connectionWrappingModel{model: model}

	// Encryption / decryption is first (outermost) before conversion to
	// native path formats.
	nm := makeNative(cwm)
	em := newEncryptedModel(nm, newFolderKeyRegistry(keyGen, passwords), keyGen)

	// We do the wire format conversion first (outermost) so that the
	// metadata is in wire format when it reaches the encryption step.
	rc := newRawConnection(deviceID, reader, writer, closer, em, connInfo, compress)
	ec := newEncryptedConnection(rc, rc, em.folderKeys, keyGen)
	wc := wireFormatConnection{ec}

	cwm.conn = wc
	return wc
}

func newRawConnection(deviceID DeviceID, reader io.Reader, writer io.Writer, closer io.Closer, receiver contextLessModel, connInfo ConnectionInfo, compress Compression) *rawConnection {
	cr := &countingReader{Reader: reader}
	cw := &countingWriter{Writer: writer}

	return &rawConnection{
		ConnectionInfo:        connInfo,
		deviceID:              deviceID,
		model:                 receiver,
		cr:                    cr,
		cw:                    cw,
		closer:                closer,
		awaiting:              make(map[int]chan asyncResult),
		inbox:                 make(chan message),
		outbox:                make(chan asyncMessage),
		closeBox:              make(chan asyncMessage),
		clusterConfigBox:      make(chan *ClusterConfig),
		dispatcherLoopStopped: make(chan struct{}),
		closed:                make(chan struct{}),
		compression:           compress,
		loopWG:                sync.WaitGroup{},
	}
}

// Start creates the goroutines for sending and receiving of messages. It must
// be called exactly once after creating a connection.
func (c *rawConnection) Start() {
	c.loopWG.Add(5)
	go func() {
		c.readerLoop()
		c.loopWG.Done()
	}()
	go func() {
		err := c.dispatcherLoop()
		c.Close(err)
		c.loopWG.Done()
	}()
	go func() {
		c.writerLoop()
		c.loopWG.Done()
	}()
	go func() {
		c.pingSender()
		c.loopWG.Done()
	}()
	go func() {
		c.pingReceiver()
		c.loopWG.Done()
	}()
	c.startTime = time.Now().Truncate(time.Second)
}

func (c *rawConnection) DeviceID() DeviceID {
	return c.deviceID
}

// Index writes the list of file information to the connected peer device
func (c *rawConnection) Index(ctx context.Context, folder string, idx []FileInfo) error {
	select {
	case <-c.closed:
		return ErrClosed
	default:
	}
	c.idxMut.Lock()
	c.send(ctx, &Index{
		Folder: folder,
		Files:  idx,
	}, nil)
	c.idxMut.Unlock()
	return nil
}

// IndexUpdate writes the list of file information to the connected peer device as an update
func (c *rawConnection) IndexUpdate(ctx context.Context, folder string, idx []FileInfo) error {
	select {
	case <-c.closed:
		return ErrClosed
	default:
	}
	c.idxMut.Lock()
	c.send(ctx, &IndexUpdate{
		Folder: folder,
		Files:  idx,
	}, nil)
	c.idxMut.Unlock()
	return nil
}

// Request returns the bytes for the specified block after fetching them from the connected peer.
func (c *rawConnection) Request(ctx context.Context, folder string, name string, blockNo int, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
	rc := make(chan asyncResult, 1)

	c.awaitingMut.Lock()
	id := c.nextID
	c.nextID++
	if _, ok := c.awaiting[id]; ok {
		c.awaitingMut.Unlock()
		panic("id taken")
	}
	c.awaiting[id] = rc
	c.awaitingMut.Unlock()

	ok := c.send(ctx, &Request{
		ID:            id,
		Folder:        folder,
		Name:          name,
		Offset:        offset,
		Size:          size,
		BlockNo:       blockNo,
		Hash:          hash,
		WeakHash:      weakHash,
		FromTemporary: fromTemporary,
	}, nil)
	if !ok {
		return nil, ErrClosed
	}

	select {
	case res, ok := <-rc:
		if !ok {
			return nil, ErrClosed
		}
		return res.val, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ClusterConfig sends the cluster configuration message to the peer.
func (c *rawConnection) ClusterConfig(config ClusterConfig) {
	select {
	case c.clusterConfigBox <- &config:
	case <-c.closed:
	}
}

func (c *rawConnection) Closed() <-chan struct{} {
	return c.closed
}

// DownloadProgress sends the progress updates for the files that are currently being downloaded.
func (c *rawConnection) DownloadProgress(ctx context.Context, folder string, updates []FileDownloadProgressUpdate) {
	c.send(ctx, &DownloadProgress{
		Folder:  folder,
		Updates: updates,
	}, nil)
}

func (c *rawConnection) ping() bool {
	return c.send(context.Background(), &Ping{}, nil)
}

func (c *rawConnection) readerLoop() {
	fourByteBuf := make([]byte, 4)
	for {
		msg, err := c.readMessage(fourByteBuf)
		if err != nil {
			if err == errUnknownMessage {
				// Unknown message types are skipped, for future extensibility.
				continue
			}
			c.internalClose(err)
			return
		}
		select {
		case c.inbox <- msg:
		case <-c.closed:
			return
		}

	}
}

func (c *rawConnection) dispatcherLoop() (err error) {
	defer close(c.dispatcherLoopStopped)
	var msg message
	state := stateInitial
	for {
		select {
		case msg = <-c.inbox:
		case <-c.closed:
			return ErrClosed
		}

		msgContext, err := messageContext(msg)
		if err != nil {
			return fmt.Errorf("protocol error: %w", err)
		}
		l.Debugf("handle %v message", msgContext)

		switch msg := msg.(type) {
		case *ClusterConfig:
			if state == stateInitial {
				state = stateReady
			}
		case *Close:
			return fmt.Errorf("closed by remote: %v", msg.Reason)
		default:
			if state != stateReady {
				return newProtocolError(fmt.Errorf("invalid state %d", state), msgContext)
			}
		}

		switch msg := msg.(type) {
		case *Index:
			err = checkIndexConsistency(msg.Files)

		case *IndexUpdate:
			err = checkIndexConsistency(msg.Files)

		case *Request:
			err = checkFilename(msg.Name)
		}
		if err != nil {
			return newProtocolError(err, msgContext)
		}

		switch msg := msg.(type) {
		case *ClusterConfig:
			err = c.model.ClusterConfig(*msg)

		case *Index:
			err = c.handleIndex(*msg)

		case *IndexUpdate:
			err = c.handleIndexUpdate(*msg)

		case *Request:
			go c.handleRequest(*msg)

		case *Response:
			c.handleResponse(*msg)

		case *DownloadProgress:
			err = c.model.DownloadProgress(msg.Folder, msg.Updates)
		}
		if err != nil {
			return newHandleError(err, msgContext)
		}
	}
}

func (c *rawConnection) readMessage(fourByteBuf []byte) (message, error) {
	hdr, err := c.readHeader(fourByteBuf)
	if err != nil {
		return nil, err
	}

	return c.readMessageAfterHeader(hdr, fourByteBuf)
}

func (c *rawConnection) readMessageAfterHeader(hdr Header, fourByteBuf []byte) (message, error) {
	// First comes a 4 byte message length

	if _, err := io.ReadFull(c.cr, fourByteBuf[:4]); err != nil {
		return nil, fmt.Errorf("reading message length: %w", err)
	}
	msgLen := int32(binary.BigEndian.Uint32(fourByteBuf))
	if msgLen < 0 {
		return nil, fmt.Errorf("negative message length %d", msgLen)
	} else if msgLen > MaxMessageLen {
		return nil, fmt.Errorf("message length %d exceeds maximum %d", msgLen, MaxMessageLen)
	}

	// Then comes the message

	buf := BufferPool.Get(int(msgLen))
	if _, err := io.ReadFull(c.cr, buf); err != nil {
		BufferPool.Put(buf)
		return nil, fmt.Errorf("reading message: %w", err)
	}

	// ... which might be compressed

	switch hdr.Compression {
	case MessageCompressionNone:
		// Nothing

	case MessageCompressionLZ4:
		decomp, err := lz4Decompress(buf)
		BufferPool.Put(buf)
		if err != nil {
			return nil, fmt.Errorf("decompressing message: %w", err)
		}
		buf = decomp

	default:
		return nil, fmt.Errorf("unknown message compression %d", hdr.Compression)
	}

	// ... and is then unmarshalled

	msg, err := newMessage(hdr.Type)
	if err != nil {
		BufferPool.Put(buf)
		return nil, err
	}
	if err := msg.Unmarshal(buf); err != nil {
		BufferPool.Put(buf)
		return nil, fmt.Errorf("unmarshalling message: %w", err)
	}
	BufferPool.Put(buf)

	return msg, nil
}

func (c *rawConnection) readHeader(fourByteBuf []byte) (Header, error) {
	// First comes a 2 byte header length

	if _, err := io.ReadFull(c.cr, fourByteBuf[:2]); err != nil {
		return Header{}, fmt.Errorf("reading length: %w", err)
	}
	hdrLen := int16(binary.BigEndian.Uint16(fourByteBuf))
	if hdrLen < 0 {
		return Header{}, fmt.Errorf("negative header length %d", hdrLen)
	}

	// Then comes the header

	buf := BufferPool.Get(int(hdrLen))
	if _, err := io.ReadFull(c.cr, buf); err != nil {
		BufferPool.Put(buf)
		return Header{}, fmt.Errorf("reading header: %w", err)
	}

	var hdr Header
	err := hdr.Unmarshal(buf)
	BufferPool.Put(buf)
	if err != nil {
		return Header{}, fmt.Errorf("unmarshalling header: %w", err)
	}

	return hdr, nil
}

func (c *rawConnection) handleIndex(im Index) error {
	l.Debugf("Index(%v, %v, %d file)", c.deviceID, im.Folder, len(im.Files))
	return c.model.Index(im.Folder, im.Files)
}

func (c *rawConnection) handleIndexUpdate(im IndexUpdate) error {
	l.Debugf("queueing IndexUpdate(%v, %v, %d files)", c.deviceID, im.Folder, len(im.Files))
	return c.model.IndexUpdate(im.Folder, im.Files)
}

// checkIndexConsistency verifies a number of invariants on FileInfos received in
// index messages.
func checkIndexConsistency(fs []FileInfo) error {
	for _, f := range fs {
		if err := checkFileInfoConsistency(f); err != nil {
			return fmt.Errorf("%q: %w", f.Name, err)
		}
	}
	return nil
}

// checkFileInfoConsistency verifies a number of invariants on the given FileInfo
func checkFileInfoConsistency(f FileInfo) error {
	if err := checkFilename(f.Name); err != nil {
		return err
	}

	switch {
	case f.Deleted && len(f.Blocks) != 0:
		// Deleted files should have no blocks
		return errDeletedHasBlocks

	case f.Type == FileInfoTypeDirectory && len(f.Blocks) != 0:
		// Directories should have no blocks
		return errDirectoryHasBlocks

	case !f.Deleted && !f.IsInvalid() && f.Type == FileInfoTypeFile && len(f.Blocks) == 0:
		// Non-deleted, non-invalid files should have at least one block
		return errFileHasNoBlocks
	}
	return nil
}

// checkFilename verifies that the given filename is valid according to the
// spec on what's allowed over the wire. A filename failing this test is
// grounds for disconnecting the device.
func checkFilename(name string) error {
	cleanedName := path.Clean(name)
	if cleanedName != name {
		// The filename on the wire should be in canonical format. If
		// Clean() managed to clean it up, there was something wrong with
		// it.
		return errUncleanFilename
	}

	switch name {
	case "", ".", "..":
		// These names are always invalid.
		return errInvalidFilename
	}
	if strings.HasPrefix(name, "/") {
		// Names are folder relative, not absolute.
		return errInvalidFilename
	}
	if strings.HasPrefix(name, "../") {
		// Starting with a dotdot is not allowed. Any other dotdots would
		// have been handled by the Clean() call at the top.
		return errInvalidFilename
	}
	return nil
}

func (c *rawConnection) handleRequest(req Request) {
	res, err := c.model.Request(req.Folder, req.Name, int32(req.BlockNo), int32(req.Size), req.Offset, req.Hash, req.WeakHash, req.FromTemporary)
	if err != nil {
		c.send(context.Background(), &Response{
			ID:   req.ID,
			Code: errorToCode(err),
		}, nil)
		return
	}
	done := make(chan struct{})
	c.send(context.Background(), &Response{
		ID:   req.ID,
		Data: res.Data(),
		Code: errorToCode(nil),
	}, done)
	<-done
	res.Close()
}

func (c *rawConnection) handleResponse(resp Response) {
	c.awaitingMut.Lock()
	if rc := c.awaiting[resp.ID]; rc != nil {
		delete(c.awaiting, resp.ID)
		rc <- asyncResult{resp.Data, codeToError(resp.Code)}
		close(rc)
	}
	c.awaitingMut.Unlock()
}

func (c *rawConnection) send(ctx context.Context, msg message, done chan struct{}) bool {
	select {
	case c.outbox <- asyncMessage{msg, done}:
		return true
	case <-c.closed:
	case <-ctx.Done():
	}
	if done != nil {
		close(done)
	}
	return false
}

func (c *rawConnection) writerLoop() {
	select {
	case cc := <-c.clusterConfigBox:
		err := c.writeMessage(cc)
		if err != nil {
			c.internalClose(err)
			return
		}
	case hm := <-c.closeBox:
		_ = c.writeMessage(hm.msg)
		close(hm.done)
		return
	case <-c.closed:
		return
	}
	for {
		select {
		case cc := <-c.clusterConfigBox:
			err := c.writeMessage(cc)
			if err != nil {
				c.internalClose(err)
				return
			}
		case hm := <-c.outbox:
			err := c.writeMessage(hm.msg)
			if hm.done != nil {
				close(hm.done)
			}
			if err != nil {
				c.internalClose(err)
				return
			}

		case hm := <-c.closeBox:
			_ = c.writeMessage(hm.msg)
			close(hm.done)
			return

		case <-c.closed:
			return
		}
	}
}

func (c *rawConnection) writeMessage(msg message) error {
	msgContext, _ := messageContext(msg)
	l.Debugf("Writing %v", msgContext)

	size := msg.ProtoSize()
	hdr := Header{
		Type: typeOf(msg),
	}
	hdrSize := hdr.ProtoSize()
	if hdrSize > 1<<16-1 {
		panic("impossibly large header")
	}

	overhead := 2 + hdrSize + 4
	totSize := overhead + size
	buf := BufferPool.Get(totSize)
	defer BufferPool.Put(buf)

	// Message
	if _, err := msg.MarshalTo(buf[2+hdrSize+4:]); err != nil {
		return fmt.Errorf("marshalling message: %w", err)
	}

	if c.shouldCompressMessage(msg) {
		ok, err := c.writeCompressedMessage(msg, buf[overhead:])
		if ok {
			return err
		}
	}

	// Header length
	binary.BigEndian.PutUint16(buf, uint16(hdrSize))
	// Header
	if _, err := hdr.MarshalTo(buf[2:]); err != nil {
		return fmt.Errorf("marshalling header: %w", err)
	}
	// Message length
	binary.BigEndian.PutUint32(buf[2+hdrSize:], uint32(size))

	n, err := c.cw.Write(buf)

	l.Debugf("wrote %d bytes on the wire (2 bytes length, %d bytes header, 4 bytes message length, %d bytes message), err=%v", n, hdrSize, size, err)
	if err != nil {
		return fmt.Errorf("writing message: %w", err)
	}
	return nil
}

// Write msg out compressed, given its uncompressed marshaled payload.
//
// The first return value indicates whether compression succeeded.
// If not, the caller should retry without compression.
func (c *rawConnection) writeCompressedMessage(msg message, marshaled []byte) (ok bool, err error) {
	hdr := Header{
		Type:        typeOf(msg),
		Compression: MessageCompressionLZ4,
	}
	hdrSize := hdr.ProtoSize()
	if hdrSize > 1<<16-1 {
		panic("impossibly large header")
	}

	cOverhead := 2 + hdrSize + 4
	// The compressed size may be at most n-n/32 = .96875*n bytes,
	// I.e., if we can't save at least 3.125% bandwidth, we forgo compression.
	// This number is arbitrary but cheap to compute.
	maxCompressed := cOverhead + len(marshaled) - len(marshaled)/32
	buf := BufferPool.Get(maxCompressed)
	defer BufferPool.Put(buf)

	compressedSize, err := lz4Compress(marshaled, buf[cOverhead:])
	totSize := compressedSize + cOverhead
	if err != nil {
		return false, nil
	}

	// Header length
	binary.BigEndian.PutUint16(buf, uint16(hdrSize))
	// Header
	if _, err := hdr.MarshalTo(buf[2:]); err != nil {
		return true, fmt.Errorf("marshalling header: %w", err)
	}
	// Message length
	binary.BigEndian.PutUint32(buf[2+hdrSize:], uint32(compressedSize))

	n, err := c.cw.Write(buf[:totSize])
	l.Debugf("wrote %d bytes on the wire (2 bytes length, %d bytes header, 4 bytes message length, %d bytes message (%d uncompressed)), err=%v", n, hdrSize, compressedSize, len(marshaled), err)
	if err != nil {
		return true, fmt.Errorf("writing message: %w", err)
	}
	return true, nil
}

func typeOf(msg message) MessageType {
	switch msg.(type) {
	case *ClusterConfig:
		return MessageTypeClusterConfig
	case *Index:
		return MessageTypeIndex
	case *IndexUpdate:
		return MessageTypeIndexUpdate
	case *Request:
		return MessageTypeRequest
	case *Response:
		return MessageTypeResponse
	case *DownloadProgress:
		return MessageTypeDownloadProgress
	case *Ping:
		return MessageTypePing
	case *Close:
		return MessageTypeClose
	default:
		panic("bug: unknown message type")
	}
}

func newMessage(t MessageType) (message, error) {
	switch t {
	case MessageTypeClusterConfig:
		return new(ClusterConfig), nil
	case MessageTypeIndex:
		return new(Index), nil
	case MessageTypeIndexUpdate:
		return new(IndexUpdate), nil
	case MessageTypeRequest:
		return new(Request), nil
	case MessageTypeResponse:
		return new(Response), nil
	case MessageTypeDownloadProgress:
		return new(DownloadProgress), nil
	case MessageTypePing:
		return new(Ping), nil
	case MessageTypeClose:
		return new(Close), nil
	default:
		return nil, errUnknownMessage
	}
}

func (c *rawConnection) shouldCompressMessage(msg message) bool {
	switch c.compression {
	case CompressionNever:
		return false

	case CompressionAlways:
		// Use compression for large enough messages
		return msg.ProtoSize() >= compressionThreshold

	case CompressionMetadata:
		_, isResponse := msg.(*Response)
		// Compress if it's large enough and not a response message
		return !isResponse && msg.ProtoSize() >= compressionThreshold

	default:
		panic("unknown compression setting")
	}
}

// Close is called when the connection is regularely closed and thus the Close
// BEP message is sent before terminating the actual connection. The error
// argument specifies the reason for closing the connection.
func (c *rawConnection) Close(err error) {
	c.sendCloseOnce.Do(func() {
		done := make(chan struct{})
		timeout := time.NewTimer(CloseTimeout)
		select {
		case c.closeBox <- asyncMessage{&Close{err.Error()}, done}:
			select {
			case <-done:
			case <-timeout.C:
			case <-c.closed:
			}
		case <-timeout.C:
		case <-c.closed:
		}
	})

	// Close might be called from a method that is called from within
	// dispatcherLoop, resulting in a deadlock.
	// The sending above must happen before spawning the routine, to prevent
	// the underlying connection from terminating before sending the close msg.
	go c.internalClose(err)
}

// internalClose is called if there is an unexpected error during normal operation.
func (c *rawConnection) internalClose(err error) {
	c.closeOnce.Do(func() {
		l.Debugln("close due to", err)
		if cerr := c.closer.Close(); cerr != nil {
			l.Debugln(c.deviceID, "failed to close underlying conn:", cerr)
		}
		close(c.closed)

		c.awaitingMut.Lock()
		for i, ch := range c.awaiting {
			if ch != nil {
				close(ch)
				delete(c.awaiting, i)
			}
		}
		c.awaitingMut.Unlock()

		<-c.dispatcherLoopStopped

		c.model.Closed(err)
	})
}

// The pingSender makes sure that we've sent a message within the last
// PingSendInterval. If we already have something sent in the last
// PingSendInterval/2, we do nothing. Otherwise we send a ping message. This
// results in an effecting ping interval of somewhere between
// PingSendInterval/2 and PingSendInterval.
func (c *rawConnection) pingSender() {
	ticker := time.NewTicker(PingSendInterval / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d := time.Since(c.cw.Last())
			if d < PingSendInterval/2 {
				l.Debugln(c.deviceID, "ping skipped after wr", d)
				continue
			}

			l.Debugln(c.deviceID, "ping -> after", d)
			c.ping()

		case <-c.closed:
			return
		}
	}
}

// The pingReceiver checks that we've received a message (any message will do,
// but we expect pings in the absence of other messages) within the last
// ReceiveTimeout. If not, we close the connection with an ErrTimeout.
func (c *rawConnection) pingReceiver() {
	ticker := time.NewTicker(ReceiveTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d := time.Since(c.cr.Last())
			if d > ReceiveTimeout {
				l.Debugln(c.deviceID, "ping timeout", d)
				c.internalClose(ErrTimeout)
			}

			l.Debugln(c.deviceID, "last read within", d)

		case <-c.closed:
			return
		}
	}
}

type Statistics struct {
	At            time.Time `json:"at"`
	InBytesTotal  int64     `json:"inBytesTotal"`
	OutBytesTotal int64     `json:"outBytesTotal"`
	StartedAt     time.Time `json:"startedAt"`
}

func (c *rawConnection) Statistics() Statistics {
	return Statistics{
		At:            time.Now().Truncate(time.Second),
		InBytesTotal:  c.cr.Tot(),
		OutBytesTotal: c.cw.Tot(),
		StartedAt:     c.startTime,
	}
}

func lz4Compress(src, buf []byte) (int, error) {
	n, err := lz4.CompressBlock(src, buf[4:], nil)
	if err != nil {
		return -1, err
	} else if n == 0 {
		return -1, errNotCompressible
	}

	// The compressed block is prefixed by the size of the uncompressed data.
	binary.BigEndian.PutUint32(buf, uint32(len(src)))

	return n + 4, nil
}

func lz4Decompress(src []byte) ([]byte, error) {
	size := binary.BigEndian.Uint32(src)
	buf := BufferPool.Get(int(size))

	n, err := lz4.UncompressBlock(src[4:], buf)
	if err != nil {
		BufferPool.Put(buf)
		return nil, err
	}

	return buf[:n], nil
}

func newProtocolError(err error, msgContext string) error {
	return fmt.Errorf("protocol error on %v: %w", msgContext, err)
}

func newHandleError(err error, msgContext string) error {
	return fmt.Errorf("handling %v: %w", msgContext, err)
}

func messageContext(msg message) (string, error) {
	switch msg := msg.(type) {
	case *ClusterConfig:
		return "cluster-config", nil
	case *Index:
		return fmt.Sprintf("index for %v", msg.Folder), nil
	case *IndexUpdate:
		return fmt.Sprintf("index-update for %v", msg.Folder), nil
	case *Request:
		return fmt.Sprintf(`request for "%v" in %v`, msg.Name, msg.Folder), nil
	case *Response:
		return "response", nil
	case *DownloadProgress:
		return fmt.Sprintf("download-progress for %v", msg.Folder), nil
	case *Ping:
		return "ping", nil
	case *Close:
		return "close", nil
	default:
		return "", errors.New("unknown or empty message")
	}
}

// connectionWrappingModel takes the Model interface from the model package,
// which expects the Connection as the first parameter in all methods, and
// wraps it to conform to the protocol.contextLessModel interface.
type connectionWrappingModel struct {
	conn  Connection
	model Model
}

func (c *connectionWrappingModel) Index(folder string, files []FileInfo) error {
	return c.model.Index(c.conn, folder, files)
}

func (c *connectionWrappingModel) IndexUpdate(folder string, files []FileInfo) error {
	return c.model.IndexUpdate(c.conn, folder, files)
}

func (c *connectionWrappingModel) Request(folder, name string, blockNo, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (RequestResponse, error) {
	return c.model.Request(c.conn, folder, name, blockNo, size, offset, hash, weakHash, fromTemporary)
}

func (c *connectionWrappingModel) ClusterConfig(config ClusterConfig) error {
	return c.model.ClusterConfig(c.conn, config)
}

func (c *connectionWrappingModel) Closed(err error) {
	c.model.Closed(c.conn, err)
}

func (c *connectionWrappingModel) DownloadProgress(folder string, updates []FileDownloadProgressUpdate) error {
	return c.model.DownloadProgress(c.conn, folder, updates)
}
