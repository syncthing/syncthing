// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package protocol

import (
	"bufio"
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
	BlockSize = 128 * 1024
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
	stateCCRcvd
	stateIdxRcvd
)

const (
	FlagDeleted    uint32 = 1 << 12
	FlagInvalid           = 1 << 13
	FlagDirectory         = 1 << 14
	FlagNoPermBits        = 1 << 15
)

const (
	FlagShareTrusted  uint32 = 1 << 0
	FlagShareReadOnly        = 1 << 1
	FlagShareBits            = 0x000000ff
)

var (
	ErrClusterHash = fmt.Errorf("configuration error: mismatched cluster hash")
	ErrClosed      = errors.New("connection closed")
)

type Model interface {
	// An index was received from the peer node
	Index(nodeID NodeID, repo string, files []FileInfo)
	// An index update was received from the peer node
	IndexUpdate(nodeID NodeID, repo string, files []FileInfo)
	// A request was made by the peer node
	Request(nodeID NodeID, repo string, name string, offset int64, size int) ([]byte, error)
	// A cluster configuration message was received
	ClusterConfig(nodeID NodeID, config ClusterConfigMessage)
	// The peer node closed the connection
	Close(nodeID NodeID, err error)
}

type Connection interface {
	ID() NodeID
	Name() string
	Index(repo string, files []FileInfo) error
	IndexUpdate(repo string, files []FileInfo) error
	Request(repo string, name string, offset int64, size int) ([]byte, error)
	ClusterConfig(config ClusterConfigMessage)
	Statistics() Statistics
}

type rawConnection struct {
	id       NodeID
	name     string
	receiver Model
	state    int

	cr *countingReader

	cw *countingWriter
	wb *bufio.Writer

	awaiting    [4096]chan asyncResult
	awaitingMut sync.Mutex

	idxMut sync.Mutex // ensures serialization of Index calls

	nextID chan int
	outbox chan hdrMsg
	closed chan struct{}
	once   sync.Once

	compressionThreshold int // compress messages larger than this many bytes

	rdbuf0 []byte // used & reused by readMessage
	rdbuf1 []byte // used & reused by readMessage
}

type asyncResult struct {
	val []byte
	err error
}

type hdrMsg struct {
	hdr header
	msg encodable
}

type encodable interface {
	AppendXDR([]byte) []byte
}

const (
	pingTimeout  = 30 * time.Second
	pingIdleTime = 60 * time.Second
)

func NewConnection(nodeID NodeID, reader io.Reader, writer io.Writer, receiver Model, name string, compress bool) Connection {
	cr := &countingReader{Reader: reader}
	cw := &countingWriter{Writer: writer}

	compThres := 1<<31 - 1 // compression disabled
	if compress {
		compThres = 128 // compress messages that are 128 bytes long or larger
	}
	c := rawConnection{
		id:                   nodeID,
		name:                 name,
		receiver:             nativeModel{receiver},
		state:                stateInitial,
		cr:                   cr,
		cw:                   cw,
		outbox:               make(chan hdrMsg),
		nextID:               make(chan int),
		closed:               make(chan struct{}),
		compressionThreshold: compThres,
	}

	go c.readerLoop()
	go c.writerLoop()
	go c.pingerLoop()
	go c.idGenerator()

	return wireFormatConnection{&c}
}

func (c *rawConnection) ID() NodeID {
	return c.id
}

func (c *rawConnection) Name() string {
	return c.name
}

// Index writes the list of file information to the connected peer node
func (c *rawConnection) Index(repo string, idx []FileInfo) error {
	select {
	case <-c.closed:
		return ErrClosed
	default:
	}
	c.idxMut.Lock()
	c.send(-1, messageTypeIndex, IndexMessage{repo, idx})
	c.idxMut.Unlock()
	return nil
}

// IndexUpdate writes the list of file information to the connected peer node as an update
func (c *rawConnection) IndexUpdate(repo string, idx []FileInfo) error {
	select {
	case <-c.closed:
		return ErrClosed
	default:
	}
	c.idxMut.Lock()
	c.send(-1, messageTypeIndexUpdate, IndexMessage{repo, idx})
	c.idxMut.Unlock()
	return nil
}

// Request returns the bytes for the specified block after fetching them from the connected peer.
func (c *rawConnection) Request(repo string, name string, offset int64, size int) ([]byte, error) {
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

	ok := c.send(id, messageTypeRequest, RequestMessage{repo, name, uint64(offset), uint32(size)})
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
	c.send(-1, messageTypeClusterConfig, config)
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

	ok := c.send(id, messageTypePing, nil)
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

		switch hdr.msgType {
		case messageTypeIndex:
			if c.state < stateCCRcvd {
				return fmt.Errorf("protocol error: index message in state %d", c.state)
			}
			c.handleIndex(msg.(IndexMessage))
			c.state = stateIdxRcvd

		case messageTypeIndexUpdate:
			if c.state < stateIdxRcvd {
				return fmt.Errorf("protocol error: index update message in state %d", c.state)
			}
			c.handleIndexUpdate(msg.(IndexMessage))

		case messageTypeRequest:
			if c.state < stateIdxRcvd {
				return fmt.Errorf("protocol error: request message in state %d", c.state)
			}
			// Requests are handled asynchronously
			go c.handleRequest(hdr.msgID, msg.(RequestMessage))

		case messageTypeResponse:
			if c.state < stateIdxRcvd {
				return fmt.Errorf("protocol error: response message in state %d", c.state)
			}
			c.handleResponse(hdr.msgID, msg.(ResponseMessage))

		case messageTypePing:
			c.send(hdr.msgID, messageTypePong, EmptyMessage{})

		case messageTypePong:
			c.handlePong(hdr.msgID)

		case messageTypeClusterConfig:
			if c.state != stateInitial {
				return fmt.Errorf("protocol error: cluster config message in state %d", c.state)
			}
			go c.receiver.ClusterConfig(c.id, msg.(ClusterConfigMessage))
			c.state = stateCCRcvd

		case messageTypeClose:
			return errors.New(msg.(CloseMessage).Reason)

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
	if hdr.compression {
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

	switch hdr.msgType {
	case messageTypeIndex, messageTypeIndexUpdate:
		var idx IndexMessage
		err = idx.UnmarshalXDR(msgBuf)
		msg = idx

	case messageTypeRequest:
		var req RequestMessage
		err = req.UnmarshalXDR(msgBuf)
		msg = req

	case messageTypeResponse:
		var resp ResponseMessage
		err = resp.UnmarshalXDR(msgBuf)
		msg = resp

	case messageTypePing, messageTypePong:
		msg = EmptyMessage{}

	case messageTypeClusterConfig:
		var cc ClusterConfigMessage
		err = cc.UnmarshalXDR(msgBuf)
		msg = cc

	case messageTypeClose:
		var cm CloseMessage
		err = cm.UnmarshalXDR(msgBuf)
		msg = cm

	default:
		err = fmt.Errorf("protocol error: %s: unknown message type %#x", c.id, hdr.msgType)
	}

	return
}

func (c *rawConnection) handleIndex(im IndexMessage) {
	if debug {
		l.Debugf("Index(%v, %v, %d files)", c.id, im.Repository, len(im.Files))
	}
	c.receiver.Index(c.id, im.Repository, im.Files)
}

func (c *rawConnection) handleIndexUpdate(im IndexMessage) {
	if debug {
		l.Debugf("queueing IndexUpdate(%v, %v, %d files)", c.id, im.Repository, len(im.Files))
	}
	c.receiver.IndexUpdate(c.id, im.Repository, im.Files)
}

func (c *rawConnection) handleRequest(msgID int, req RequestMessage) {
	data, _ := c.receiver.Request(c.id, req.Repository, req.Name, int64(req.Offset), int(req.Size))

	c.send(msgID, messageTypeResponse, ResponseMessage{data})
}

func (c *rawConnection) handleResponse(msgID int, resp ResponseMessage) {
	c.awaitingMut.Lock()
	if rc := c.awaiting[msgID]; rc != nil {
		c.awaiting[msgID] = nil
		rc <- asyncResult{resp.Data, nil}
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

func (c *rawConnection) send(msgID int, msgType int, msg encodable) bool {
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
	case c.outbox <- hdrMsg{hdr, msg}:
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
				uncBuf = hm.msg.AppendXDR(uncBuf[:0])

				if len(uncBuf) >= c.compressionThreshold {
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
	ticker := time.Tick(pingIdleTime / 2)
	for {
		select {
		case <-ticker:
			if d := time.Since(c.cr.Last()); d < pingIdleTime {
				if debug {
					l.Debugln(c.id, "ping skipped after rd", d)
				}
				continue
			}
			if d := time.Since(c.cw.Last()); d < pingIdleTime {
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
			case <-time.After(pingTimeout):
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
	InBytesTotal  uint64
	OutBytesTotal uint64
}

func (c *rawConnection) Statistics() Statistics {
	return Statistics{
		At:            time.Now(),
		InBytesTotal:  c.cr.Tot(),
		OutBytesTotal: c.cw.Tot(),
	}
}

func IsDeleted(bits uint32) bool {
	return bits&FlagDeleted != 0
}

func IsInvalid(bits uint32) bool {
	return bits&FlagInvalid != 0
}

func IsDirectory(bits uint32) bool {
	return bits&FlagDirectory != 0
}

func HasPermissionBits(bits uint32) bool {
	return bits&FlagNoPermBits == 0
}
