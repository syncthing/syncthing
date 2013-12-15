package protocol

import (
	"compress/flate"
	"errors"
	"io"
	"sync"

	"github.com/calmh/syncthing/buffers"
)

const (
	messageTypeReserved = iota
	messageTypeIndex
	messageTypeRequest
	messageTypeResponse
	messageTypePing
	messageTypePong
)

type FileInfo struct {
	Name     string
	Flags    uint32
	Modified int64
	Blocks   []BlockInfo
}

type BlockInfo struct {
	Length uint32
	Hash   []byte
}

type Model interface {
	// An index was received from the peer node
	Index(nodeID string, files []FileInfo)
	// A request was made by the peer node
	Request(nodeID, name string, offset uint64, size uint32, hash []byte) ([]byte, error)
	// The peer node closed the connection
	Close(nodeID string)
}

type Connection struct {
	receiver   Model
	reader     io.Reader
	mreader    *marshalReader
	writer     io.Writer
	mwriter    *marshalWriter
	wLock      sync.RWMutex
	closed     bool
	closedLock sync.RWMutex
	awaiting   map[int]chan asyncResult
	nextId     int
	ID         string
}

var ErrClosed = errors.New("Connection closed")

type asyncResult struct {
	val []byte
	err error
}

func NewConnection(nodeID string, reader io.Reader, writer io.Writer, receiver Model) *Connection {
	flrd := flate.NewReader(reader)
	flwr, err := flate.NewWriter(writer, flate.BestSpeed)
	if err != nil {
		panic(err)
	}

	c := Connection{
		receiver: receiver,
		reader:   flrd,
		mreader:  &marshalReader{flrd, 0, nil},
		writer:   flwr,
		mwriter:  &marshalWriter{flwr, 0, nil},
		awaiting: make(map[int]chan asyncResult),
		ID:       nodeID,
	}

	go c.readerLoop()

	return &c
}

// Index writes the list of file information to the connected peer node
func (c *Connection) Index(idx []FileInfo) {
	c.wLock.Lock()
	defer c.wLock.Unlock()

	c.mwriter.writeHeader(header{0, c.nextId, messageTypeIndex})
	c.nextId = (c.nextId + 1) & 0xfff
	c.mwriter.writeIndex(idx)
	c.flush()
}

// Request returns the bytes for the specified block after fetching them from the connected peer.
func (c *Connection) Request(name string, offset uint64, size uint32, hash []byte) ([]byte, error) {
	c.wLock.Lock()
	rc := make(chan asyncResult)
	c.awaiting[c.nextId] = rc
	c.mwriter.writeHeader(header{0, c.nextId, messageTypeRequest})
	c.mwriter.writeRequest(request{name, offset, size, hash})
	c.flush()
	c.nextId = (c.nextId + 1) & 0xfff
	c.wLock.Unlock()

	res, ok := <-rc
	if !ok {
		return nil, ErrClosed
	}
	return res.val, res.err
}

func (c *Connection) Ping() bool {
	c.wLock.Lock()
	rc := make(chan asyncResult)
	c.awaiting[c.nextId] = rc
	c.mwriter.writeHeader(header{0, c.nextId, messageTypePing})
	c.flush()
	c.nextId = (c.nextId + 1) & 0xfff
	c.wLock.Unlock()

	_, ok := <-rc
	return ok
}

func (c *Connection) Stop() {
}

type flusher interface {
	Flush() error
}

func (c *Connection) flush() {
	if f, ok := c.writer.(flusher); ok {
		f.Flush()
	}
}

func (c *Connection) close() {
	c.closedLock.Lock()
	c.closed = true
	c.closedLock.Unlock()

	c.wLock.Lock()
	for _, ch := range c.awaiting {
		close(ch)
	}
	c.awaiting = nil
	c.wLock.Unlock()

	c.receiver.Close(c.ID)
}

func (c *Connection) isClosed() bool {
	c.closedLock.RLock()
	defer c.closedLock.RUnlock()
	return c.closed
}

func (c *Connection) readerLoop() {
	for !c.isClosed() {
		hdr := c.mreader.readHeader()
		if c.mreader.err != nil {
			c.close()
			break
		}

		switch hdr.msgType {
		case messageTypeIndex:
			files := c.mreader.readIndex()
			if c.mreader.err != nil {
				c.close()
			} else {
				c.receiver.Index(c.ID, files)
			}

		case messageTypeRequest:
			c.processRequest(hdr.msgID)

		case messageTypeResponse:
			data := c.mreader.readResponse()

			if c.mreader.err != nil {
				c.close()
			} else {
				c.wLock.RLock()
				rc, ok := c.awaiting[hdr.msgID]
				c.wLock.RUnlock()

				if ok {
					rc <- asyncResult{data, c.mreader.err}
					close(rc)

					c.wLock.Lock()
					delete(c.awaiting, hdr.msgID)
					c.wLock.Unlock()
				}
			}

		case messageTypePing:
			c.wLock.Lock()
			c.mwriter.writeUint32(encodeHeader(header{0, hdr.msgID, messageTypePong}))
			c.flush()
			c.wLock.Unlock()

		case messageTypePong:
			c.wLock.RLock()
			rc, ok := c.awaiting[hdr.msgID]
			c.wLock.RUnlock()

			if ok {
				rc <- asyncResult{}
				close(rc)

				c.wLock.Lock()
				delete(c.awaiting, hdr.msgID)
				c.wLock.Unlock()
			}
		}
	}
}

func (c *Connection) processRequest(msgID int) {
	req := c.mreader.readRequest()
	if c.mreader.err != nil {
		c.close()
	} else {
		go func() {
			data, _ := c.receiver.Request(c.ID, req.name, req.offset, req.size, req.hash)
			c.wLock.Lock()
			c.mwriter.writeUint32(encodeHeader(header{0, msgID, messageTypeResponse}))
			c.mwriter.writeResponse(data)
			buffers.Put(data)
			c.flush()
			c.wLock.Unlock()
		}()
	}
}
