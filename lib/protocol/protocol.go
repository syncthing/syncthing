// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:generate -command counterfeiter go run github.com/maxbrunsfeld/counterfeiter/v6

// Prevents import loop, for internal testing
//go:generate counterfeiter -o mocked_connection_info_test.go --fake-name mockedConnectionInfo . ConnectionInfo
//go:generate go run ../../script/prune_mocks.go -t mocked_connection_info_test.go

//go:generate counterfeiter -o mocks/connection_info.go --fake-name ConnectionInfo . ConnectionInfo
//go:generate counterfeiter -o mocks/connection.go --fake-name Connection . Connection

package protocol

import (
	"context"
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
	"google.golang.org/protobuf/proto"

	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/internal/protoutil"
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

	SyntheticDirectorySize = 128

	// don't bother compressing messages smaller than this many bytes
	compressionThreshold = 128
)

var errNotCompressible = errors.New("not compressible")

const (
	stateInitial = iota
	stateReady
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

type Model interface {
	// An index was received from the peer device
	Index(conn Connection, idx *Index) error
	// An index update was received from the peer device
	IndexUpdate(conn Connection, idxUp *IndexUpdate) error
	// A request was made by the peer device
	Request(conn Connection, req *Request) (RequestResponse, error)
	// A cluster configuration message was received
	ClusterConfig(conn Connection, config *ClusterConfig) error
	// The peer device closed the connection or an error occurred
	Closed(conn Connection, err error)
	// The peer device sent progress updates for the files it is currently downloading
	DownloadProgress(conn Connection, p *DownloadProgress) error
}

// rawModel is the Model interface, but without the initial Connection
// parameter. Internal use only.
type rawModel interface {
	Index(*Index) error
	IndexUpdate(*IndexUpdate) error
	Request(*Request) (RequestResponse, error)
	ClusterConfig(*ClusterConfig) error
	Closed(err error)
	DownloadProgress(*DownloadProgress) error
}

type RequestResponse interface {
	Data() []byte
	Close() // Must always be called once the byte slice is no longer in use
	Wait()  // Blocks until Close is called
}

type Connection interface {
	// Send an Index message to the peer device. The message in the
	// parameter may be altered by the connection and should not be used
	// further by the caller.
	Index(ctx context.Context, idx *Index) error

	// Send an Index Update message to the peer device. The message in the
	// parameter may be altered by the connection and should not be used
	// further by the caller.
	IndexUpdate(ctx context.Context, idxUp *IndexUpdate) error

	// Send a Request message to the peer device. The message in the
	// parameter may be altered by the connection and should not be used
	// further by the caller.
	Request(ctx context.Context, req *Request) ([]byte, error)

	// Send a Cluster Configuration message to the peer device. The message
	// in the parameter may be altered by the connection and should not be
	// used further by the caller.
	ClusterConfig(config *ClusterConfig)

	// Send a Download Progress message to the peer device. The message in
	// the parameter may be altered by the connection and should not be used
	// further by the caller.
	DownloadProgress(ctx context.Context, dp *DownloadProgress)

	Start()
	SetFolderPasswords(passwords map[string]string)
	Close(err error)
	DeviceID() DeviceID
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
	ConnectionID() string
}

type rawConnection struct {
	ConnectionInfo

	deviceID  DeviceID
	idString  string
	model     rawModel
	startTime time.Time
	started   chan struct{}

	cr     *countingReader
	cw     *countingWriter
	closer io.Closer // Closing the underlying connection and thus cr and cw

	awaitingMut sync.Mutex // Protects awaiting and nextID.
	awaiting    map[int]chan asyncResult
	nextID      int

	idxMut sync.Mutex // ensures serialization of Index calls

	inbox                 chan proto.Message
	outbox                chan asyncMessage
	closeBox              chan asyncMessage
	clusterConfigBox      chan *ClusterConfig
	dispatcherLoopStopped chan struct{}
	closed                chan struct{}
	closeOnce             sync.Once
	sendCloseOnce         sync.Once
	compression           Compression
	startStopMut          sync.Mutex // start and stop must be serialized

	loopWG sync.WaitGroup // Need to ensure no leftover routines in testing
}

type asyncResult struct {
	val []byte
	err error
}

type asyncMessage struct {
	msg  proto.Message
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

func newRawConnection(deviceID DeviceID, reader io.Reader, writer io.Writer, closer io.Closer, receiver rawModel, connInfo ConnectionInfo, compress Compression) *rawConnection {
	idString := deviceID.String()
	cr := &countingReader{Reader: reader, idString: idString}
	cw := &countingWriter{Writer: writer, idString: idString}
	registerDeviceMetrics(idString)

	return &rawConnection{
		ConnectionInfo:        connInfo,
		deviceID:              deviceID,
		idString:              deviceID.String(),
		model:                 receiver,
		started:               make(chan struct{}),
		cr:                    cr,
		cw:                    cw,
		closer:                closer,
		awaiting:              make(map[int]chan asyncResult),
		inbox:                 make(chan proto.Message),
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
	c.startStopMut.Lock()
	defer c.startStopMut.Unlock()
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
	close(c.started)
}

func (c *rawConnection) DeviceID() DeviceID {
	return c.deviceID
}

// Index writes the list of file information to the connected peer device
func (c *rawConnection) Index(ctx context.Context, idx *Index) error {
	select {
	case <-c.closed:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.idxMut.Lock()
	c.send(ctx, idx.toWire(), nil)
	c.idxMut.Unlock()
	return nil
}

// IndexUpdate writes the list of file information to the connected peer device as an update
func (c *rawConnection) IndexUpdate(ctx context.Context, idxUp *IndexUpdate) error {
	select {
	case <-c.closed:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.idxMut.Lock()
	c.send(ctx, idxUp.toWire(), nil)
	c.idxMut.Unlock()
	return nil
}

// Request returns the bytes for the specified block after fetching them from the connected peer.
func (c *rawConnection) Request(ctx context.Context, req *Request) ([]byte, error) {
	select {
	case <-c.closed:
		return nil, ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

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

	req.ID = id
	ok := c.send(ctx, req.toWire(), nil)
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
func (c *rawConnection) ClusterConfig(config *ClusterConfig) {
	select {
	case c.clusterConfigBox <- config:
	case <-c.closed:
	}
}

func (c *rawConnection) Closed() <-chan struct{} {
	return c.closed
}

// DownloadProgress sends the progress updates for the files that are currently being downloaded.
func (c *rawConnection) DownloadProgress(ctx context.Context, dp *DownloadProgress) {
	c.send(ctx, dp.toWire(), nil)
}

func (c *rawConnection) ping() bool {
	return c.send(context.Background(), &bep.Ping{}, nil)
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
	var msg proto.Message
	state := stateInitial
	for {
		select {
		case <-c.closed:
			return ErrClosed
		default:
		}
		select {
		case msg = <-c.inbox:
		case <-c.closed:
			return ErrClosed
		}

		metricDeviceRecvMessages.WithLabelValues(c.idString).Inc()

		msgContext, err := messageContext(msg)
		if err != nil {
			return fmt.Errorf("protocol error: %w", err)
		}
		l.Debugf("handle %v message", msgContext)

		switch msg := msg.(type) {
		case *bep.ClusterConfig:
			if state == stateInitial {
				state = stateReady
			}
		case *bep.Close:
			return fmt.Errorf("closed by remote: %v", msg.Reason)
		default:
			if state != stateReady {
				return newProtocolError(fmt.Errorf("invalid state %d", state), msgContext)
			}
		}

		switch msg := msg.(type) {
		case *bep.Request:
			err = checkFilename(msg.Name)
		}
		if err != nil {
			return newProtocolError(err, msgContext)
		}

		switch msg := msg.(type) {
		case *bep.ClusterConfig:
			err = c.model.ClusterConfig(clusterConfigFromWire(msg))

		case *bep.Index:
			idx := indexFromWire(msg)
			if err := checkIndexConsistency(idx.Files); err != nil {
				return newProtocolError(err, msgContext)
			}
			err = c.handleIndex(idx)

		case *bep.IndexUpdate:
			idxUp := indexUpdateFromWire(msg)
			if err := checkIndexConsistency(idxUp.Files); err != nil {
				return newProtocolError(err, msgContext)
			}
			err = c.handleIndexUpdate(idxUp)

		case *bep.Request:
			go c.handleRequest(requestFromWire(msg))

		case *bep.Response:
			c.handleResponse(responseFromWire(msg))

		case *bep.DownloadProgress:
			err = c.model.DownloadProgress(downloadProgressFromWire(msg))
		}
		if err != nil {
			return newHandleError(err, msgContext)
		}
	}
}

func (c *rawConnection) readMessage(fourByteBuf []byte) (proto.Message, error) {
	hdr, err := c.readHeader(fourByteBuf)
	if err != nil {
		return nil, err
	}

	return c.readMessageAfterHeader(hdr, fourByteBuf)
}

func (c *rawConnection) readMessageAfterHeader(hdr *bep.Header, fourByteBuf []byte) (proto.Message, error) {
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
	case bep.MessageCompression_MESSAGE_COMPRESSION_NONE:
		// Nothing

	case bep.MessageCompression_MESSAGE_COMPRESSION_LZ4:
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

	metricDeviceRecvDecompressedBytes.WithLabelValues(c.idString).Add(float64(4 + len(buf)))

	msg, err := newMessage(hdr.Type)
	if err != nil {
		BufferPool.Put(buf)
		return nil, err
	}
	if err := proto.Unmarshal(buf, msg); err != nil {
		BufferPool.Put(buf)
		return nil, fmt.Errorf("unmarshalling message: %w", err)
	}
	BufferPool.Put(buf)

	return msg, nil
}

func (c *rawConnection) readHeader(fourByteBuf []byte) (*bep.Header, error) {
	// First comes a 2 byte header length

	if _, err := io.ReadFull(c.cr, fourByteBuf[:2]); err != nil {
		return nil, fmt.Errorf("reading length: %w", err)
	}
	hdrLen := int16(binary.BigEndian.Uint16(fourByteBuf))
	if hdrLen < 0 {
		return nil, fmt.Errorf("negative header length %d", hdrLen)
	}

	// Then comes the header

	buf := BufferPool.Get(int(hdrLen))
	if _, err := io.ReadFull(c.cr, buf); err != nil {
		BufferPool.Put(buf)
		return nil, fmt.Errorf("reading header: %w", err)
	}

	var hdr bep.Header
	err := proto.Unmarshal(buf, &hdr)
	BufferPool.Put(buf)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling header: %w %x", err, buf)
	}

	metricDeviceRecvDecompressedBytes.WithLabelValues(c.idString).Add(float64(2 + len(buf)))

	return &hdr, nil
}

func (c *rawConnection) handleIndex(im *Index) error {
	l.Debugf("Index(%v, %v, %d file)", c.deviceID, im.Folder, len(im.Files))
	return c.model.Index(im)
}

func (c *rawConnection) handleIndexUpdate(im *IndexUpdate) error {
	l.Debugf("queueing IndexUpdate(%v, %v, %d files)", c.deviceID, im.Folder, len(im.Files))
	return c.model.IndexUpdate(im)
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

func (c *rawConnection) handleRequest(req *Request) {
	res, err := c.model.Request(req)
	if err != nil {
		resp := &Response{
			ID:   req.ID,
			Code: errorToCode(err),
		}
		c.send(context.Background(), resp.toWire(), nil)
		return
	}
	done := make(chan struct{})
	resp := &Response{
		ID:   req.ID,
		Data: res.Data(),
		Code: errorToCode(nil),
	}
	c.send(context.Background(), resp.toWire(), done)
	<-done
	res.Close()
}

func (c *rawConnection) handleResponse(resp *Response) {
	c.awaitingMut.Lock()
	if rc := c.awaiting[resp.ID]; rc != nil {
		delete(c.awaiting, resp.ID)
		rc <- asyncResult{resp.Data, codeToError(resp.Code)}
		close(rc)
	}
	c.awaitingMut.Unlock()
}

func (c *rawConnection) send(ctx context.Context, msg proto.Message, done chan struct{}) bool {
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
		err := c.writeMessage(cc.toWire())
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
		// When the connection is closing or closed, that should happen
		// immediately, not compete with the (potentially very busy) outbox.
		select {
		case hm := <-c.closeBox:
			_ = c.writeMessage(hm.msg)
			close(hm.done)
			return
		case <-c.closed:
			return
		default:
		}
		select {
		case cc := <-c.clusterConfigBox:
			err := c.writeMessage(cc.toWire())
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

func (c *rawConnection) writeMessage(msg proto.Message) error {
	msgContext, _ := messageContext(msg)
	l.Debugf("Writing %v", msgContext)

	defer func() {
		metricDeviceSentMessages.WithLabelValues(c.idString).Inc()
	}()

	size := proto.Size(msg)
	hdr := &bep.Header{
		Type: typeOf(msg),
	}
	hdrSize := proto.Size(hdr)
	if hdrSize > 1<<16-1 {
		panic("impossibly large header")
	}

	overhead := 2 + hdrSize + 4
	totSize := overhead + size
	buf := BufferPool.Get(totSize)
	defer BufferPool.Put(buf)

	// Message
	if _, err := protoutil.MarshalTo(buf[overhead:], msg); err != nil {
		return fmt.Errorf("marshalling message: %w", err)
	}

	if c.shouldCompressMessage(msg) {
		ok, err := c.writeCompressedMessage(msg, buf[overhead:])
		if ok {
			return err
		}
	}

	metricDeviceSentUncompressedBytes.WithLabelValues(c.idString).Add(float64(totSize))

	// Header length
	binary.BigEndian.PutUint16(buf, uint16(hdrSize))
	// Header
	if _, err := protoutil.MarshalTo(buf[2:], hdr); err != nil {
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
func (c *rawConnection) writeCompressedMessage(msg proto.Message, marshaled []byte) (ok bool, err error) {
	hdr := &bep.Header{
		Type:        typeOf(msg),
		Compression: bep.MessageCompression_MESSAGE_COMPRESSION_LZ4,
	}
	hdrSize := proto.Size(hdr)
	if hdrSize > 1<<16-1 {
		panic("impossibly large header")
	}

	cOverhead := 2 + hdrSize + 4

	metricDeviceSentUncompressedBytes.WithLabelValues(c.idString).Add(float64(cOverhead + len(marshaled)))

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
	if _, err := protoutil.MarshalTo(buf[2:], hdr); err != nil {
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

func typeOf(msg proto.Message) bep.MessageType {
	switch msg.(type) {
	case *bep.ClusterConfig:
		return bep.MessageType_MESSAGE_TYPE_CLUSTER_CONFIG
	case *bep.Index:
		return bep.MessageType_MESSAGE_TYPE_INDEX
	case *bep.IndexUpdate:
		return bep.MessageType_MESSAGE_TYPE_INDEX_UPDATE
	case *bep.Request:
		return bep.MessageType_MESSAGE_TYPE_REQUEST
	case *bep.Response:
		return bep.MessageType_MESSAGE_TYPE_RESPONSE
	case *bep.DownloadProgress:
		return bep.MessageType_MESSAGE_TYPE_DOWNLOAD_PROGRESS
	case *bep.Ping:
		return bep.MessageType_MESSAGE_TYPE_PING
	case *bep.Close:
		return bep.MessageType_MESSAGE_TYPE_CLOSE
	default:
		panic("bug: unknown message type")
	}
}

func newMessage(t bep.MessageType) (proto.Message, error) {
	switch t {
	case bep.MessageType_MESSAGE_TYPE_CLUSTER_CONFIG:
		return new(bep.ClusterConfig), nil
	case bep.MessageType_MESSAGE_TYPE_INDEX:
		return new(bep.Index), nil
	case bep.MessageType_MESSAGE_TYPE_INDEX_UPDATE:
		return new(bep.IndexUpdate), nil
	case bep.MessageType_MESSAGE_TYPE_REQUEST:
		return new(bep.Request), nil
	case bep.MessageType_MESSAGE_TYPE_RESPONSE:
		return new(bep.Response), nil
	case bep.MessageType_MESSAGE_TYPE_DOWNLOAD_PROGRESS:
		return new(bep.DownloadProgress), nil
	case bep.MessageType_MESSAGE_TYPE_PING:
		return new(bep.Ping), nil
	case bep.MessageType_MESSAGE_TYPE_CLOSE:
		return new(bep.Close), nil
	default:
		return nil, errUnknownMessage
	}
}

func (c *rawConnection) shouldCompressMessage(msg proto.Message) bool {
	switch c.compression {
	case CompressionNever:
		return false

	case CompressionAlways:
		// Use compression for large enough messages
		return proto.Size(msg) >= compressionThreshold

	case CompressionMetadata:
		_, isResponse := msg.(*bep.Response)
		// Compress if it's large enough and not a response message
		return !isResponse && proto.Size(msg) >= compressionThreshold

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
		case c.closeBox <- asyncMessage{&bep.Close{Reason: err.Error()}, done}:
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
	c.startStopMut.Lock()
	defer c.startStopMut.Unlock()
	c.closeOnce.Do(func() {
		l.Debugf("close connection to %s at %s due to %v", c.deviceID.Short(), c.ConnectionInfo, err)
		if cerr := c.closer.Close(); cerr != nil {
			l.Debugf("failed to close underlying conn %s at %s %v:", c.deviceID.Short(), c.ConnectionInfo, cerr)
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

		if !c.startTime.IsZero() {
			// Wait for the dispatcher loop to exit, if it was started to
			// begin with.
			<-c.dispatcherLoopStopped
		}

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

func messageContext(msg proto.Message) (string, error) {
	switch msg := msg.(type) {
	case *bep.ClusterConfig:
		return "cluster-config", nil
	case *bep.Index:
		return fmt.Sprintf("index for %v", msg.Folder), nil
	case *bep.IndexUpdate:
		return fmt.Sprintf("index-update for %v", msg.Folder), nil
	case *bep.Request:
		return fmt.Sprintf(`request for "%v" in %v`, msg.Name, msg.Folder), nil
	case *bep.Response:
		return "response", nil
	case *bep.DownloadProgress:
		return fmt.Sprintf("download-progress for %v", msg.Folder), nil
	case *bep.Ping:
		return "ping", nil
	case *bep.Close:
		return "close", nil
	default:
		return "", errors.New("unknown or empty message")
	}
}

// connectionWrappingModel takes the Model interface from the model package,
// which expects the Connection as the first parameter in all methods, and
// wraps it to conform to the rawModel interface.
type connectionWrappingModel struct {
	conn  Connection
	model Model
}

func (c *connectionWrappingModel) Index(m *Index) error {
	return c.model.Index(c.conn, m)
}

func (c *connectionWrappingModel) IndexUpdate(idxUp *IndexUpdate) error {
	return c.model.IndexUpdate(c.conn, idxUp)
}

func (c *connectionWrappingModel) Request(req *Request) (RequestResponse, error) {
	return c.model.Request(c.conn, req)
}

func (c *connectionWrappingModel) ClusterConfig(config *ClusterConfig) error {
	return c.model.ClusterConfig(c.conn, config)
}

func (c *connectionWrappingModel) Closed(err error) {
	c.model.Closed(c.conn, err)
}

func (c *connectionWrappingModel) DownloadProgress(p *DownloadProgress) error {
	return c.model.DownloadProgress(c.conn, p)
}
