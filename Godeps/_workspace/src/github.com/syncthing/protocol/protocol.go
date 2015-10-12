// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	lz4 "github.com/bkaradzic/go-lz4"
)

const (
	// Data block size (128 KiB)
	BlockSize = 128 << 10

	// We reject messages larger than this when encountered on the wire. (64 MiB)
	MaxMessageLen = 64 << 20
)

const (
	messageTypeClusterConfig = 0
	messageTypeIndex         = 1
	messageTypeRequest       = 2
	messageTypeResponse      = 3
	messageTypePing          = 4
	messageTypePong          = 5
	messageTypeIndexUpdate   = 6
	messageTypeClose         = 7
)

const (
	stateInitial = iota
	stateReady
)

// FileInfo flags
const (
	FlagDeleted              uint32 = 1 << 12
	FlagInvalid                     = 1 << 13
	FlagDirectory                   = 1 << 14
	FlagNoPermBits                  = 1 << 15
	FlagSymlink                     = 1 << 16
	FlagSymlinkMissingTarget        = 1 << 17

	FlagsAll = (1 << 18) - 1

	SymlinkTypeMask = FlagDirectory | FlagSymlinkMissingTarget
)

// IndexMessage message flags (for IndexUpdate)
const (
	FlagIndexTemporary uint32 = 1 << iota
)

// Request message flags
const (
	FlagRequestTemporary uint32 = 1 << iota
)

// ClusterConfigMessage.Folders.Devices flags
const (
	FlagShareTrusted  uint32 = 1 << 0
	FlagShareReadOnly        = 1 << 1
	FlagIntroducer           = 1 << 2
	FlagShareBits            = 0x000000ff
)

var (
	ErrClusterHash = fmt.Errorf("configuration error: mismatched cluster hash")
	ErrClosed      = errors.New("connection closed")
)

// Specific variants of empty messages...
type pingMessage struct{ EmptyMessage }
type pongMessage struct{ EmptyMessage }

type Model interface {
	// An index was received from the peer device
	Index(deviceID DeviceID, folder string, files []FileInfo, flags uint32, options []Option)
	// An index update was received from the peer device
	IndexUpdate(deviceID DeviceID, folder string, files []FileInfo, flags uint32, options []Option)
	// A request was made by the peer device
	Request(deviceID DeviceID, folder string, name string, offset int64, hash []byte, flags uint32, options []Option, buf []byte) error
	// A cluster configuration message was received
	ClusterConfig(deviceID DeviceID, config ClusterConfigMessage)
	// The peer device closed the connection
	Close(deviceID DeviceID, err error)
}

type Connection interface {
	Start()
	ID() DeviceID
	Name() string
	Index(folder string, files []FileInfo, flags uint32, options []Option) error
	IndexUpdate(folder string, files []FileInfo, flags uint32, options []Option) error
	Request(folder string, name string, offset int64, size int, hash []byte, flags uint32, options []Option) ([]byte, error)
	ClusterConfig(config ClusterConfigMessage)
	Statistics() Statistics
}

type rawConnection struct {
	id       DeviceID
	name     string
	receiver Model

	cr *countingReader
	cw *countingWriter

	awaiting    [4096]chan asyncResult
	awaitingMut sync.Mutex

	idxMut sync.Mutex // ensures serialization of Index calls

	nextID      chan int
	outbox      chan hdrMsg
	closed      chan struct{}
	once        sync.Once
	pool        sync.Pool
	compression Compression

	rdbuf0 []byte // used & reused by readMessage
	rdbuf1 []byte // used & reused by readMessage
}

type asyncResult struct {
	val []byte
	err error
}

type hdrMsg struct {
	hdr  header
	msg  encodable
	done chan struct{}
}

type encodable interface {
	AppendXDR([]byte) ([]byte, error)
}

type isEofer interface {
	IsEOF() bool
}

var (
	PingTimeout  = 30 * time.Second
	PingIdleTime = 60 * time.Second
)

func NewConnection(deviceID DeviceID, reader io.Reader, writer io.Writer, receiver Model, name string, compress Compression) Connection {
	cr := &countingReader{Reader: reader}
	cw := &countingWriter{Writer: writer}

	c := rawConnection{
		id:       deviceID,
		name:     name,
		receiver: nativeModel{receiver},
		cr:       cr,
		cw:       cw,
		outbox:   make(chan hdrMsg),
		nextID:   make(chan int),
		closed:   make(chan struct{}),
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, BlockSize)
			},
		},
		compression: compress,
	}

	return wireFormatConnection{&c}
}

// Start creates the goroutines for sending and receiving of messages. It must
// be called exactly once after creating a connection.
func (c *rawConnection) Start() {
	go c.readerLoop()
	go c.writerLoop()
	go c.pingerLoop()
	go c.idGenerator()
}

func (c *rawConnection) ID() DeviceID {
	return c.id
}

func (c *rawConnection) Name() string {
	return c.name
}

// Index writes the list of file information to the connected peer device
func (c *rawConnection) Index(folder string, idx []FileInfo, flags uint32, options []Option) error {
	select {
	case <-c.closed:
		return ErrClosed
	default:
	}
	c.idxMut.Lock()
	c.send(-1, messageTypeIndex, IndexMessage{
		Folder:  folder,
		Files:   idx,
		Flags:   flags,
		Options: options,
	}, nil)
	c.idxMut.Unlock()
	return nil
}

// IndexUpdate writes the list of file information to the connected peer device as an update
func (c *rawConnection) IndexUpdate(folder string, idx []FileInfo, flags uint32, options []Option) error {
	select {
	case <-c.closed:
		return ErrClosed
	default:
	}
	c.idxMut.Lock()
	c.send(-1, messageTypeIndexUpdate, IndexMessage{
		Folder:  folder,
		Files:   idx,
		Flags:   flags,
		Options: options,
	}, nil)
	c.idxMut.Unlock()
	return nil
}

// Request returns the bytes for the specified block after fetching them from the connected peer.
func (c *rawConnection) Request(folder string, name string, offset int64, size int, hash []byte, flags uint32, options []Option) ([]byte, error) {
	var id int
	select {
	case id = <-c.nextID:
	case <-c.closed:
		return nil, ErrClosed
	}

	c.awaitingMut.Lock()
	if ch := c.awaiting[id]; ch != nil {
		panic("id taken")
	}
	rc := make(chan asyncResult, 1)
	c.awaiting[id] = rc
	c.awaitingMut.Unlock()

	ok := c.send(id, messageTypeRequest, RequestMessage{
		Folder:  folder,
		Name:    name,
		Offset:  offset,
		Size:    int32(size),
		Hash:    hash,
		Flags:   flags,
		Options: options,
	}, nil)
	if !ok {
		return nil, ErrClosed
	}

	res, ok := <-rc
	if !ok {
		return nil, ErrClosed
	}
	return res.val, res.err
}

// ClusterConfig send the cluster configuration message to the peer and returns any error
func (c *rawConnection) ClusterConfig(config ClusterConfigMessage) {
	c.send(-1, messageTypeClusterConfig, config, nil)
}

func (c *rawConnection) ping() bool {
	var id int
	select {
	case id = <-c.nextID:
	case <-c.closed:
		return false
	}

	rc := make(chan asyncResult, 1)
	c.awaitingMut.Lock()
	c.awaiting[id] = rc
	c.awaitingMut.Unlock()

	ok := c.send(id, messageTypePing, nil, nil)
	if !ok {
		return false
	}

	res, ok := <-rc
	return ok && res.err == nil
}

func (c *rawConnection) readerLoop() (err error) {
	defer func() {
		c.close(err)
	}()

	state := stateInitial
	for {
		select {
		case <-c.closed:
			return ErrClosed
		default:
		}

		hdr, msg, err := c.readMessage()
		if err != nil {
			return err
		}

		switch msg := msg.(type) {
		case ClusterConfigMessage:
			if state != stateInitial {
				return fmt.Errorf("protocol error: cluster config message in state %d", state)
			}
			go c.receiver.ClusterConfig(c.id, msg)
			state = stateReady

		case IndexMessage:
			switch hdr.msgType {
			case messageTypeIndex:
				if state != stateReady {
					return fmt.Errorf("protocol error: index message in state %d", state)
				}
				c.handleIndex(msg)
				state = stateReady

			case messageTypeIndexUpdate:
				if state != stateReady {
					return fmt.Errorf("protocol error: index update message in state %d", state)
				}
				c.handleIndexUpdate(msg)
				state = stateReady
			}

		case RequestMessage:
			if state != stateReady {
				return fmt.Errorf("protocol error: request message in state %d", state)
			}
			// Requests are handled asynchronously
			go c.handleRequest(hdr.msgID, msg)

		case ResponseMessage:
			if state != stateReady {
				return fmt.Errorf("protocol error: response message in state %d", state)
			}
			c.handleResponse(hdr.msgID, msg)

		case pingMessage:
			if state != stateReady {
				return fmt.Errorf("protocol error: ping message in state %d", state)
			}
			c.send(hdr.msgID, messageTypePong, pongMessage{}, nil)

		case pongMessage:
			if state != stateReady {
				return fmt.Errorf("protocol error: pong message in state %d", state)
			}
			c.handlePong(hdr.msgID)

		case CloseMessage:
			return errors.New(msg.Reason)

		default:
			return fmt.Errorf("protocol error: %s: unknown message type %#x", c.id, hdr.msgType)
		}
	}
}

func (c *rawConnection) readMessage() (hdr header, msg encodable, err error) {
	if cap(c.rdbuf0) < 8 {
		c.rdbuf0 = make([]byte, 8)
	} else {
		c.rdbuf0 = c.rdbuf0[:8]
	}
	_, err = io.ReadFull(c.cr, c.rdbuf0)
	if err != nil {
		return
	}

	hdr = decodeHeader(binary.BigEndian.Uint32(c.rdbuf0[0:4]))
	msglen := int(binary.BigEndian.Uint32(c.rdbuf0[4:8]))

	if debug {
		l.Debugf("read header %v (msglen=%d)", hdr, msglen)
	}

	if msglen > MaxMessageLen {
		err = fmt.Errorf("message length %d exceeds maximum %d", msglen, MaxMessageLen)
		return
	}

	if hdr.version != 0 {
		err = fmt.Errorf("unknown protocol version 0x%x", hdr.version)
		return
	}

	if cap(c.rdbuf0) < msglen {
		c.rdbuf0 = make([]byte, msglen)
	} else {
		c.rdbuf0 = c.rdbuf0[:msglen]
	}
	_, err = io.ReadFull(c.cr, c.rdbuf0)
	if err != nil {
		return
	}

	if debug {
		l.Debugf("read %d bytes", len(c.rdbuf0))
	}

	msgBuf := c.rdbuf0
	if hdr.compression && msglen > 0 {
		c.rdbuf1 = c.rdbuf1[:cap(c.rdbuf1)]
		c.rdbuf1, err = lz4.Decode(c.rdbuf1, c.rdbuf0)
		if err != nil {
			return
		}
		msgBuf = c.rdbuf1
		if debug {
			l.Debugf("decompressed to %d bytes", len(msgBuf))
		}
	}

	if debug {
		if len(msgBuf) > 1024 {
			l.Debugf("message data:\n%s", hex.Dump(msgBuf[:1024]))
		} else {
			l.Debugf("message data:\n%s", hex.Dump(msgBuf))
		}
	}

	// We check each returned error for the XDRError.IsEOF() method.
	// IsEOF()==true here means that the message contained fewer fields than
	// expected. It does not signify an EOF on the socket, because we've
	// successfully read a size value and that many bytes already. New fields
	// we expected but the other peer didn't send should be interpreted as
	// zero/nil, and if that's not valid we'll verify it somewhere else.

	switch hdr.msgType {
	case messageTypeIndex, messageTypeIndexUpdate:
		var idx IndexMessage
		err = idx.UnmarshalXDR(msgBuf)
		if xdrErr, ok := err.(isEofer); ok && xdrErr.IsEOF() {
			err = nil
		}
		msg = idx

	case messageTypeRequest:
		var req RequestMessage
		err = req.UnmarshalXDR(msgBuf)
		if xdrErr, ok := err.(isEofer); ok && xdrErr.IsEOF() {
			err = nil
		}
		msg = req

	case messageTypeResponse:
		var resp ResponseMessage
		err = resp.UnmarshalXDR(msgBuf)
		if xdrErr, ok := err.(isEofer); ok && xdrErr.IsEOF() {
			err = nil
		}
		msg = resp

	case messageTypePing:
		msg = pingMessage{}

	case messageTypePong:
		msg = pongMessage{}

	case messageTypeClusterConfig:
		var cc ClusterConfigMessage
		err = cc.UnmarshalXDR(msgBuf)
		if xdrErr, ok := err.(isEofer); ok && xdrErr.IsEOF() {
			err = nil
		}
		msg = cc

	case messageTypeClose:
		var cm CloseMessage
		err = cm.UnmarshalXDR(msgBuf)
		if xdrErr, ok := err.(isEofer); ok && xdrErr.IsEOF() {
			err = nil
		}
		msg = cm

	default:
		err = fmt.Errorf("protocol error: %s: unknown message type %#x", c.id, hdr.msgType)
	}

	return
}

func (c *rawConnection) handleIndex(im IndexMessage) {
	if debug {
		l.Debugf("Index(%v, %v, %d file, flags %x, opts: %s)", c.id, im.Folder, len(im.Files), im.Flags, im.Options)
	}
	c.receiver.Index(c.id, im.Folder, filterIndexMessageFiles(im.Files), im.Flags, im.Options)
}

func (c *rawConnection) handleIndexUpdate(im IndexMessage) {
	if debug {
		l.Debugf("queueing IndexUpdate(%v, %v, %d files, flags %x, opts: %s)", c.id, im.Folder, len(im.Files), im.Flags, im.Options)
	}
	c.receiver.IndexUpdate(c.id, im.Folder, filterIndexMessageFiles(im.Files), im.Flags, im.Options)
}

func filterIndexMessageFiles(fs []FileInfo) []FileInfo {
	var out []FileInfo
	for i, f := range fs {
		switch f.Name {
		case "", ".", "..", "/": // A few obviously invalid filenames
			l.Infof("Dropping invalid filename %q from incoming index", f.Name)
			if out == nil {
				// Most incoming updates won't contain anything invalid, so we
				// delay the allocation and copy to output slice until we
				// really need to do it, then copy all the so var valid files
				// to it.
				out = make([]FileInfo, i, len(fs)-1)
				copy(out, fs)
			}
		default:
			if out != nil {
				out = append(out, f)
			}
		}
	}
	if out != nil {
		return out
	}
	return fs
}

func (c *rawConnection) handleRequest(msgID int, req RequestMessage) {
	size := int(req.Size)
	usePool := size <= BlockSize

	var buf []byte
	var done chan struct{}

	if usePool {
		buf = c.pool.Get().([]byte)[:size]
		done = make(chan struct{})
	} else {
		buf = make([]byte, size)
	}

	err := c.receiver.Request(c.id, req.Folder, req.Name, int64(req.Offset), req.Hash, req.Flags, req.Options, buf)
	if err != nil {
		c.send(msgID, messageTypeResponse, ResponseMessage{
			Data: nil,
			Code: errorToCode(err),
		}, done)
	} else {
		c.send(msgID, messageTypeResponse, ResponseMessage{
			Data: buf,
			Code: errorToCode(err),
		}, done)
	}

	if usePool {
		<-done
		c.pool.Put(buf)
	}
}

func (c *rawConnection) handleResponse(msgID int, resp ResponseMessage) {
	c.awaitingMut.Lock()
	if rc := c.awaiting[msgID]; rc != nil {
		c.awaiting[msgID] = nil
		rc <- asyncResult{resp.Data, codeToError(resp.Code)}
		close(rc)
	}
	c.awaitingMut.Unlock()
}

func (c *rawConnection) handlePong(msgID int) {
	c.awaitingMut.Lock()
	if rc := c.awaiting[msgID]; rc != nil {
		c.awaiting[msgID] = nil
		rc <- asyncResult{}
		close(rc)
	}
	c.awaitingMut.Unlock()
}

func (c *rawConnection) send(msgID int, msgType int, msg encodable, done chan struct{}) bool {
	if msgID < 0 {
		select {
		case id := <-c.nextID:
			msgID = id
		case <-c.closed:
			return false
		}
	}

	hdr := header{
		version: 0,
		msgID:   msgID,
		msgType: msgType,
	}

	select {
	case c.outbox <- hdrMsg{hdr, msg, done}:
		return true
	case <-c.closed:
		return false
	}
}

func (c *rawConnection) writerLoop() {
	var msgBuf = make([]byte, 8) // buffer for wire format message, kept and reused
	var uncBuf []byte            // buffer for uncompressed message, kept and reused
	for {
		var tempBuf []byte
		var err error

		select {
		case hm := <-c.outbox:
			if hm.msg != nil {
				// Uncompressed message in uncBuf
				uncBuf, err = hm.msg.AppendXDR(uncBuf[:0])
				if hm.done != nil {
					close(hm.done)
				}
				if err != nil {
					c.close(err)
					return
				}

				compress := false
				switch c.compression {
				case CompressAlways:
					compress = true
				case CompressMetadata:
					compress = hm.hdr.msgType != messageTypeResponse
				}

				if compress && len(uncBuf) >= compressionThreshold {
					// Use compression for large messages
					hm.hdr.compression = true

					// Make sure we have enough space for the compressed message plus header in msgBug
					msgBuf = msgBuf[:cap(msgBuf)]
					if maxLen := lz4.CompressBound(len(uncBuf)) + 8; maxLen > len(msgBuf) {
						msgBuf = make([]byte, maxLen)
					}

					// Compressed is written to msgBuf, we keep tb for the length only
					tempBuf, err = lz4.Encode(msgBuf[8:], uncBuf)
					binary.BigEndian.PutUint32(msgBuf[4:8], uint32(len(tempBuf)))
					msgBuf = msgBuf[0 : len(tempBuf)+8]

					if debug {
						l.Debugf("write compressed message; %v (len=%d)", hm.hdr, len(tempBuf))
					}
				} else {
					// No point in compressing very short messages
					hm.hdr.compression = false

					msgBuf = msgBuf[:cap(msgBuf)]
					if l := len(uncBuf) + 8; l > len(msgBuf) {
						msgBuf = make([]byte, l)
					}

					binary.BigEndian.PutUint32(msgBuf[4:8], uint32(len(uncBuf)))
					msgBuf = msgBuf[0 : len(uncBuf)+8]
					copy(msgBuf[8:], uncBuf)

					if debug {
						l.Debugf("write uncompressed message; %v (len=%d)", hm.hdr, len(uncBuf))
					}
				}
			} else {
				if debug {
					l.Debugf("write empty message; %v", hm.hdr)
				}
				binary.BigEndian.PutUint32(msgBuf[4:8], 0)
				msgBuf = msgBuf[:8]
			}

			binary.BigEndian.PutUint32(msgBuf[0:4], encodeHeader(hm.hdr))

			if err == nil {
				var n int
				n, err = c.cw.Write(msgBuf)
				if debug {
					l.Debugf("wrote %d bytes on the wire", n)
				}
			}
			if err != nil {
				c.close(err)
				return
			}
		case <-c.closed:
			return
		}
	}
}

func (c *rawConnection) close(err error) {
	c.once.Do(func() {
		close(c.closed)

		c.awaitingMut.Lock()
		for i, ch := range c.awaiting {
			if ch != nil {
				close(ch)
				c.awaiting[i] = nil
			}
		}
		c.awaitingMut.Unlock()

		go c.receiver.Close(c.id, err)
	})
}

func (c *rawConnection) idGenerator() {
	nextID := 0
	for {
		nextID = (nextID + 1) & 0xfff
		select {
		case c.nextID <- nextID:
		case <-c.closed:
			return
		}
	}
}

func (c *rawConnection) pingerLoop() {
	var rc = make(chan bool, 1)
	ticker := time.Tick(PingIdleTime / 2)
	for {
		select {
		case <-ticker:
			if d := time.Since(c.cr.Last()); d < PingIdleTime {
				if debug {
					l.Debugln(c.id, "ping skipped after rd", d)
				}
				continue
			}
			if d := time.Since(c.cw.Last()); d < PingIdleTime {
				if debug {
					l.Debugln(c.id, "ping skipped after wr", d)
				}
				continue
			}
			go func() {
				if debug {
					l.Debugln(c.id, "ping ->")
				}
				rc <- c.ping()
			}()
			select {
			case ok := <-rc:
				if debug {
					l.Debugln(c.id, "<- pong")
				}
				if !ok {
					c.close(fmt.Errorf("ping failure"))
				}
			case <-time.After(PingTimeout):
				c.close(fmt.Errorf("ping timeout"))
			case <-c.closed:
				return
			}

		case <-c.closed:
			return
		}
	}
}

type Statistics struct {
	At            time.Time
	InBytesTotal  int64
	OutBytesTotal int64
}

func (c *rawConnection) Statistics() Statistics {
	return Statistics{
		At:            time.Now(),
		InBytesTotal:  c.cr.Tot(),
		OutBytesTotal: c.cw.Tot(),
	}
}
