package protocol

import (
	"compress/flate"
	"errors"
	"io"
	"log"
	"sync"
	"time"

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
	ID             string
	receiver       Model
	reader         io.Reader
	mreader        *marshalReader
	writer         io.Writer
	mwriter        *marshalWriter
	wLock          sync.RWMutex
	closed         bool
	awaiting       map[int]chan asyncResult
	nextId         int
	lastReceive    time.Time
	peerLatency    time.Duration
	lastStatistics Statistics
}

var ErrClosed = errors.New("Connection closed")

type asyncResult struct {
	val []byte
	err error
}

const pingTimeout = 30 * time.Second
const pingIdleTime = 5 * time.Minute

func NewConnection(nodeID string, reader io.Reader, writer io.Writer, receiver Model) *Connection {
	flrd := flate.NewReader(reader)
	flwr, err := flate.NewWriter(writer, flate.BestSpeed)
	if err != nil {
		panic(err)
	}

	c := Connection{
		receiver:       receiver,
		reader:         flrd,
		mreader:        &marshalReader{flrd, 0, nil},
		writer:         flwr,
		mwriter:        &marshalWriter{flwr, 0, nil},
		awaiting:       make(map[int]chan asyncResult),
		lastReceive:    time.Now(),
		ID:             nodeID,
		lastStatistics: Statistics{At: time.Now()},
	}

	go c.readerLoop()
	go c.pingerLoop()

	return &c
}

// Index writes the list of file information to the connected peer node
func (c *Connection) Index(idx []FileInfo) {
	c.wLock.Lock()
	c.mwriter.writeHeader(header{0, c.nextId, messageTypeIndex})
	c.mwriter.writeIndex(idx)
	err := c.flush()
	c.nextId = (c.nextId + 1) & 0xfff
	c.wLock.Unlock()
	if err != nil || c.mwriter.err != nil {
		c.close()
		return
	}
}

// Request returns the bytes for the specified block after fetching them from the connected peer.
func (c *Connection) Request(name string, offset uint64, size uint32, hash []byte) ([]byte, error) {
	c.wLock.Lock()
	rc := make(chan asyncResult)
	c.awaiting[c.nextId] = rc
	c.mwriter.writeHeader(header{0, c.nextId, messageTypeRequest})
	c.mwriter.writeRequest(request{name, offset, size, hash})
	if c.mwriter.err != nil {
		c.wLock.Unlock()
		c.close()
		return nil, c.mwriter.err
	}
	err := c.flush()
	if err != nil {
		c.wLock.Unlock()
		c.close()
		return nil, err
	}
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
	err := c.flush()
	if err != nil || c.mwriter.err != nil {
		c.wLock.Unlock()
		c.close()
		return false
	}
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

func (c *Connection) flush() error {
	if f, ok := c.writer.(flusher); ok {
		return f.Flush()
	}
	return nil
}

func (c *Connection) close() {
	c.wLock.Lock()
	if c.closed {
		c.wLock.Unlock()
		return
	}
	c.closed = true
	for _, ch := range c.awaiting {
		close(ch)
	}
	c.awaiting = nil
	c.wLock.Unlock()

	c.receiver.Close(c.ID)
}

func (c *Connection) isClosed() bool {
	c.wLock.RLock()
	defer c.wLock.RUnlock()
	return c.closed
}

func (c *Connection) readerLoop() {
	for !c.isClosed() {
		hdr := c.mreader.readHeader()
		if c.mreader.err != nil {
			c.close()
			break
		}
		if hdr.version != 0 {
			log.Printf("Protocol error: %s: unknown message version %#x", c.ID, hdr.version)
			c.close()
			break
		}

		c.wLock.Lock()
		c.lastReceive = time.Now()
		c.wLock.Unlock()

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
			if c.mreader.err != nil || c.mwriter.err != nil {
				c.close()
			}

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
			err := c.flush()
			c.wLock.Unlock()
			if err != nil || c.mwriter.err != nil {
				c.close()
			}

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

		default:
			log.Printf("Protocol error: %s: unknown message type %#x", c.ID, hdr.msgType)
			c.close()
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
			err := c.flush()
			c.wLock.Unlock()
			buffers.Put(data)
			if c.mwriter.err != nil || err != nil {
				c.close()
			}
		}()
	}
}

func (c *Connection) pingerLoop() {
	var rc = make(chan time.Duration)
	for !c.isClosed() {
		c.wLock.RLock()
		lr := c.lastReceive
		c.wLock.RUnlock()

		if time.Since(lr) > pingIdleTime {
			go func() {
				t0 := time.Now()
				c.Ping()
				rc <- time.Since(t0)
			}()
			select {
			case lat := <-rc:
				c.wLock.Lock()
				c.peerLatency = (c.peerLatency + lat) / 2
				c.wLock.Unlock()
			case <-time.After(pingTimeout):
				c.close()
			}
		}
		time.Sleep(time.Second)
	}
}

type Statistics struct {
	At             time.Time
	InBytesTotal   int
	InBytesPerSec  int
	OutBytesTotal  int
	OutBytesPerSec int
	Latency        time.Duration
}

func (c *Connection) Statistics() Statistics {
	c.wLock.Lock()
	defer c.wLock.Unlock()

	secs := time.Since(c.lastStatistics.At).Seconds()
	stats := Statistics{
		At:             time.Now(),
		InBytesTotal:   c.mreader.tot,
		InBytesPerSec:  int(float64(c.mreader.tot-c.lastStatistics.InBytesTotal) / secs),
		OutBytesTotal:  c.mwriter.tot,
		OutBytesPerSec: int(float64(c.mwriter.tot-c.lastStatistics.OutBytesTotal) / secs),
		Latency:        c.peerLatency,
	}
	c.lastStatistics = stats
	return stats
}
