// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package protocol

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/calmh/syncthing/xdr"
)

const BlockSize = 128 * 1024

const (
	messageTypeClusterConfig = 0
	messageTypeIndex         = 1
	messageTypeRequest       = 2
	messageTypeResponse      = 3
	messageTypePing          = 4
	messageTypePong          = 5
	messageTypeIndexUpdate   = 6
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
	xr *xdr.Reader

	cw *countingWriter
	wb *bufio.Writer
	xw *xdr.Writer

	awaiting    []chan asyncResult
	awaitingMut sync.Mutex

	idxMut sync.Mutex // ensures serialization of Index calls

	nextID chan int
	outbox chan []encodable
	closed chan struct{}
	once   sync.Once
}

type asyncResult struct {
	val []byte
	err error
}

const (
	pingTimeout  = 30 * time.Second
	pingIdleTime = 60 * time.Second
)

func NewConnection(nodeID NodeID, reader io.Reader, writer io.Writer, receiver Model, name string) Connection {
	cr := &countingReader{Reader: reader}
	cw := &countingWriter{Writer: writer}

	rb := bufio.NewReader(cr)
	wb := bufio.NewWriterSize(cw, 65536)

	c := rawConnection{
		id:       nodeID,
		name:     name,
		receiver: nativeModel{receiver},
		state:    stateInitial,
		cr:       cr,
		xr:       xdr.NewReader(rb),
		cw:       cw,
		wb:       wb,
		xw:       xdr.NewWriter(wb),
		awaiting: make([]chan asyncResult, 0x1000),
		outbox:   make(chan []encodable),
		nextID:   make(chan int),
		closed:   make(chan struct{}),
	}

	go c.indexSerializerLoop()
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
	c.send(header{0, -1, messageTypeIndex}, IndexMessage{repo, idx})
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
	c.send(header{0, -1, messageTypeIndexUpdate}, IndexMessage{repo, idx})
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

	ok := c.send(header{0, id, messageTypeRequest},
		RequestMessage{repo, name, uint64(offset), uint32(size)})
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
	c.send(header{0, -1, messageTypeClusterConfig}, config)
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

	ok := c.send(header{0, id, messageTypePing})
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

		var hdr header
		hdr.decodeXDR(c.xr)
		if err := c.xr.Error(); err != nil {
			return err
		}
		if hdr.version != 0 {
			return fmt.Errorf("protocol error: %s: unknown message version %#x", c.id, hdr.version)
		}

		switch hdr.msgType {
		case messageTypeIndex:
			if c.state < stateCCRcvd {
				return fmt.Errorf("protocol error: index message in state %d", c.state)
			}
			if err := c.handleIndex(); err != nil {
				return err
			}
			c.state = stateIdxRcvd

		case messageTypeIndexUpdate:
			if c.state < stateIdxRcvd {
				return fmt.Errorf("protocol error: index update message in state %d", c.state)
			}
			if err := c.handleIndexUpdate(); err != nil {
				return err
			}

		case messageTypeRequest:
			if c.state < stateIdxRcvd {
				return fmt.Errorf("protocol error: request message in state %d", c.state)
			}
			if err := c.handleRequest(hdr); err != nil {
				return err
			}

		case messageTypeResponse:
			if c.state < stateIdxRcvd {
				return fmt.Errorf("protocol error: response message in state %d", c.state)
			}
			if err := c.handleResponse(hdr); err != nil {
				return err
			}

		case messageTypePing:
			c.send(header{0, hdr.msgID, messageTypePong})

		case messageTypePong:
			c.handlePong(hdr)

		case messageTypeClusterConfig:
			if c.state != stateInitial {
				return fmt.Errorf("protocol error: cluster config message in state %d", c.state)
			}
			if err := c.handleClusterConfig(); err != nil {
				return err
			}
			c.state = stateCCRcvd

		default:
			return fmt.Errorf("protocol error: %s: unknown message type %#x", c.id, hdr.msgType)
		}
	}
}

type incomingIndex struct {
	update bool
	id     NodeID
	repo   string
	files  []FileInfo
}

var incomingIndexes = make(chan incomingIndex, 100) // should be enough for anyone, right?

func (c *rawConnection) indexSerializerLoop() {
	// We must avoid blocking the reader loop when processing large indexes.
	// There is otherwise a potential deadlock where both sides has the model
	// locked because it's sending a large index update and can't receive the
	// large index update from the other side. But we must also ensure to
	// process the indexes in the order they are received, hence the separate
	// routine and buffered channel.
	for {
		select {
		case ii := <-incomingIndexes:
			if ii.update {
				c.receiver.IndexUpdate(ii.id, ii.repo, ii.files)
			} else {
				c.receiver.Index(ii.id, ii.repo, ii.files)
			}
		case <-c.closed:
			return
		}
	}
}

func (c *rawConnection) handleIndex() error {
	var im IndexMessage
	im.decodeXDR(c.xr)
	if err := c.xr.Error(); err != nil {
		return err
	} else {

		// We run this (and the corresponding one for update, below)
		// in a separate goroutine to avoid blocking the read loop.
		// There is otherwise a potential deadlock where both sides
		// has the model locked because it's sending a large index
		// update and can't receive the large index update from the
		// other side.

		incomingIndexes <- incomingIndex{false, c.id, im.Repository, im.Files}
	}
	return nil
}

func (c *rawConnection) handleIndexUpdate() error {
	var im IndexMessage
	im.decodeXDR(c.xr)
	if err := c.xr.Error(); err != nil {
		return err
	} else {
		incomingIndexes <- incomingIndex{true, c.id, im.Repository, im.Files}
	}
	return nil
}

func (c *rawConnection) handleRequest(hdr header) error {
	var req RequestMessage
	req.decodeXDR(c.xr)
	if err := c.xr.Error(); err != nil {
		return err
	}
	go c.processRequest(hdr.msgID, req)
	return nil
}

func (c *rawConnection) handleResponse(hdr header) error {
	data := c.xr.ReadBytesMax(256 * 1024) // Sufficiently larger than max expected block size

	if err := c.xr.Error(); err != nil {
		return err
	}

	c.awaitingMut.Lock()
	if rc := c.awaiting[hdr.msgID]; rc != nil {
		c.awaiting[hdr.msgID] = nil
		rc <- asyncResult{data, nil}
		close(rc)
	}
	c.awaitingMut.Unlock()

	return nil
}

func (c *rawConnection) handlePong(hdr header) {
	c.awaitingMut.Lock()
	if rc := c.awaiting[hdr.msgID]; rc != nil {
		c.awaiting[hdr.msgID] = nil
		rc <- asyncResult{}
		close(rc)
	}
	c.awaitingMut.Unlock()
}

func (c *rawConnection) handleClusterConfig() error {
	var cm ClusterConfigMessage
	cm.decodeXDR(c.xr)
	if err := c.xr.Error(); err != nil {
		return err
	} else {
		go c.receiver.ClusterConfig(c.id, cm)
	}
	return nil
}

type encodable interface {
	encodeXDR(*xdr.Writer) (int, error)
}
type encodableBytes []byte

func (e encodableBytes) encodeXDR(xw *xdr.Writer) (int, error) {
	return xw.WriteBytes(e)
}

func (c *rawConnection) send(h header, es ...encodable) bool {
	if h.msgID < 0 {
		select {
		case id := <-c.nextID:
			h.msgID = id
		case <-c.closed:
			return false
		}
	}
	msg := append([]encodable{h}, es...)

	select {
	case c.outbox <- msg:
		return true
	case <-c.closed:
		return false
	}
}

func (c *rawConnection) writerLoop() {
	for {
		select {
		case es := <-c.outbox:
			for _, e := range es {
				e.encodeXDR(c.xw)
			}
			if err := c.flush(); err != nil {
				c.close(err)
				return
			}
		case <-c.closed:
			return
		}
	}
}

type flusher interface {
	Flush() error
}

func (c *rawConnection) flush() error {
	if err := c.xw.Error(); err != nil {
		return err
	}
	if err := c.wb.Flush(); err != nil {
		return err
	}
	return nil
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
			if d := time.Since(c.xr.LastRead()); d < pingIdleTime {
				if debug {
					l.Debugln(c.id, "ping skipped after rd", d)
				}
				continue
			}
			if d := time.Since(c.xw.LastWrite()); d < pingIdleTime {
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

func (c *rawConnection) processRequest(msgID int, req RequestMessage) {
	data, _ := c.receiver.Request(c.id, req.Repository, req.Name, int64(req.Offset), int(req.Size))

	c.send(header{0, msgID, messageTypeResponse}, encodableBytes(data))
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
