package protocol

import (
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/calmh/syncthing/buffers"
)

const (
	messageTypeIndex       = 1
	messageTypeRequest     = 2
	messageTypeResponse    = 3
	messageTypePing        = 4
	messageTypePong        = 5
	messageTypeIndexUpdate = 6
	messageTypeOptions     = 7
)

const (
	FlagDeleted = 1 << 12
	FlagInvalid = 1 << 13
)

type FileInfo struct {
	Name     string
	Flags    uint32
	Modified int64
	Version  uint32
	Blocks   []BlockInfo
}

type BlockInfo struct {
	Size uint32
	Hash []byte
}

type Model interface {
	// An index was received from the peer node
	Index(nodeID string, files []FileInfo)
	// An index update was received from the peer node
	IndexUpdate(nodeID string, files []FileInfo)
	// A request was made by the peer node
	Request(nodeID, name string, offset int64, size uint32, hash []byte) ([]byte, error)
	// The peer node closed the connection
	Close(nodeID string, err error)
}

type Connection struct {
	sync.RWMutex

	id          string
	receiver    Model
	reader      io.Reader
	mreader     *marshalReader
	writer      io.Writer
	mwriter     *marshalWriter
	closed      bool
	awaiting    map[int]chan asyncResult
	nextId      int
	indexSent   map[string][2]int64
	options     map[string]string
	optionsLock sync.Mutex

	hasSentIndex  bool
	hasRecvdIndex bool

	statisticsLock sync.Mutex
}

var ErrClosed = errors.New("Connection closed")

type asyncResult struct {
	val []byte
	err error
}

const (
	pingTimeout  = 2 * time.Minute
	pingIdleTime = 5 * time.Minute
)

func NewConnection(nodeID string, reader io.Reader, writer io.Writer, receiver Model, options map[string]string) *Connection {
	flrd := flate.NewReader(reader)
	flwr, err := flate.NewWriter(writer, flate.BestSpeed)
	if err != nil {
		panic(err)
	}

	c := Connection{
		id:       nodeID,
		receiver: receiver,
		reader:   flrd,
		mreader:  &marshalReader{r: flrd},
		writer:   flwr,
		mwriter:  &marshalWriter{w: flwr},
		awaiting: make(map[int]chan asyncResult),
	}

	go c.readerLoop()
	go c.pingerLoop()

	if options != nil {
		go func() {
			c.Lock()
			c.mwriter.writeHeader(header{0, c.nextId, messageTypeOptions})
			c.mwriter.writeOptions(options)
			err := c.flush()
			if err != nil {
				log.Println("Warning: Write error during initial handshake:", err)
			}
			c.nextId++
			c.Unlock()
		}()
	}

	return &c
}

func (c *Connection) ID() string {
	return c.id
}

// Index writes the list of file information to the connected peer node
func (c *Connection) Index(idx []FileInfo) {
	c.Lock()
	var msgType int
	if c.indexSent == nil {
		// This is the first time we send an index.
		msgType = messageTypeIndex

		c.indexSent = make(map[string][2]int64)
		for _, f := range idx {
			c.indexSent[f.Name] = [2]int64{f.Modified, int64(f.Version)}
		}
	} else {
		// We have sent one full index. Only send updates now.
		msgType = messageTypeIndexUpdate
		var diff []FileInfo
		for _, f := range idx {
			if vs, ok := c.indexSent[f.Name]; !ok || f.Modified != vs[0] || int64(f.Version) != vs[1] {
				diff = append(diff, f)
				c.indexSent[f.Name] = [2]int64{f.Modified, int64(f.Version)}
			}
		}
		idx = diff
	}

	c.mwriter.writeHeader(header{0, c.nextId, msgType})
	c.mwriter.writeIndex(idx)
	err := c.flush()
	c.nextId = (c.nextId + 1) & 0xfff
	c.hasSentIndex = true
	c.Unlock()

	if err != nil {
		c.close(err)
		return
	} else if c.mwriter.err != nil {
		c.close(c.mwriter.err)
		return
	}
}

// Request returns the bytes for the specified block after fetching them from the connected peer.
func (c *Connection) Request(name string, offset int64, size uint32, hash []byte) ([]byte, error) {
	c.Lock()
	if c.closed {
		c.Unlock()
		return nil, ErrClosed
	}
	rc := make(chan asyncResult)
	c.awaiting[c.nextId] = rc
	c.mwriter.writeHeader(header{0, c.nextId, messageTypeRequest})
	c.mwriter.writeRequest(request{name, offset, size, hash})
	if c.mwriter.err != nil {
		c.Unlock()
		c.close(c.mwriter.err)
		return nil, c.mwriter.err
	}
	err := c.flush()
	if err != nil {
		c.Unlock()
		c.close(err)
		return nil, err
	}
	c.nextId = (c.nextId + 1) & 0xfff
	c.Unlock()

	res, ok := <-rc
	if !ok {
		return nil, ErrClosed
	}
	return res.val, res.err
}

func (c *Connection) ping() bool {
	c.Lock()
	if c.closed {
		c.Unlock()
		return false
	}
	rc := make(chan asyncResult, 1)
	c.awaiting[c.nextId] = rc
	c.mwriter.writeHeader(header{0, c.nextId, messageTypePing})
	err := c.flush()
	if err != nil {
		c.Unlock()
		c.close(err)
		return false
	} else if c.mwriter.err != nil {
		c.Unlock()
		c.close(c.mwriter.err)
		return false
	}
	c.nextId = (c.nextId + 1) & 0xfff
	c.Unlock()

	res, ok := <-rc
	return ok && res.err == nil
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

func (c *Connection) close(err error) {
	c.Lock()
	if c.closed {
		c.Unlock()
		return
	}
	c.closed = true
	for _, ch := range c.awaiting {
		close(ch)
	}
	c.awaiting = nil
	c.Unlock()

	c.receiver.Close(c.id, err)
}

func (c *Connection) isClosed() bool {
	c.RLock()
	defer c.RUnlock()
	return c.closed
}

func (c *Connection) readerLoop() {
loop:
	for {
		hdr := c.mreader.readHeader()
		if c.mreader.err != nil {
			c.close(c.mreader.err)
			break loop
		}
		if hdr.version != 0 {
			c.close(fmt.Errorf("Protocol error: %s: unknown message version %#x", c.ID, hdr.version))
			break loop
		}

		switch hdr.msgType {
		case messageTypeIndex:
			files := c.mreader.readIndex()
			if c.mreader.err != nil {
				c.close(c.mreader.err)
				break loop
			} else {
				c.receiver.Index(c.id, files)
			}
			c.Lock()
			c.hasRecvdIndex = true
			c.Unlock()

		case messageTypeIndexUpdate:
			files := c.mreader.readIndex()
			if c.mreader.err != nil {
				c.close(c.mreader.err)
				break loop
			} else {
				c.receiver.IndexUpdate(c.id, files)
			}

		case messageTypeRequest:
			req := c.mreader.readRequest()
			if c.mreader.err != nil {
				c.close(c.mreader.err)
				break loop
			}
			go c.processRequest(hdr.msgID, req)

		case messageTypeResponse:
			data := c.mreader.readResponse()

			if c.mreader.err != nil {
				c.close(c.mreader.err)
				break loop
			} else {
				c.Lock()
				rc, ok := c.awaiting[hdr.msgID]
				delete(c.awaiting, hdr.msgID)
				c.Unlock()

				if ok {
					rc <- asyncResult{data, c.mreader.err}
					close(rc)
				}
			}

		case messageTypePing:
			c.Lock()
			c.mwriter.writeUint32(encodeHeader(header{0, hdr.msgID, messageTypePong}))
			err := c.flush()
			c.Unlock()
			if err != nil {
				c.close(err)
				break loop
			} else if c.mwriter.err != nil {
				c.close(c.mwriter.err)
				break loop
			}

		case messageTypePong:
			c.RLock()
			rc, ok := c.awaiting[hdr.msgID]
			c.RUnlock()

			if ok {
				rc <- asyncResult{}
				close(rc)

				c.Lock()
				delete(c.awaiting, hdr.msgID)
				c.Unlock()
			}

		case messageTypeOptions:
			c.optionsLock.Lock()
			c.options = c.mreader.readOptions()
			c.optionsLock.Unlock()

		default:
			c.close(fmt.Errorf("Protocol error: %s: unknown message type %#x", c.ID, hdr.msgType))
			break loop
		}
	}
}

func (c *Connection) processRequest(msgID int, req request) {
	data, _ := c.receiver.Request(c.id, req.name, req.offset, req.size, req.hash)

	c.Lock()
	c.mwriter.writeUint32(encodeHeader(header{0, msgID, messageTypeResponse}))
	c.mwriter.writeResponse(data)
	err := c.mwriter.err
	if err == nil {
		err = c.flush()
	}
	c.Unlock()

	buffers.Put(data)
	if err != nil {
		c.close(err)
	}
}

func (c *Connection) pingerLoop() {
	var rc = make(chan bool, 1)
	for {
		time.Sleep(pingIdleTime / 2)

		c.RLock()
		ready := c.hasRecvdIndex && c.hasSentIndex
		c.RUnlock()

		if ready {
			go func() {
				rc <- c.ping()
			}()
			select {
			case ok := <-rc:
				if !ok {
					c.close(fmt.Errorf("Ping failure"))
				}
			case <-time.After(pingTimeout):
				c.close(fmt.Errorf("Ping timeout"))
			}
		}
	}
}

type Statistics struct {
	At            time.Time
	InBytesTotal  int
	OutBytesTotal int
}

func (c *Connection) Statistics() Statistics {
	c.statisticsLock.Lock()
	defer c.statisticsLock.Unlock()

	stats := Statistics{
		At:            time.Now(),
		InBytesTotal:  int(c.mreader.getTot()),
		OutBytesTotal: int(c.mwriter.getTot()),
	}

	return stats
}

func (c *Connection) Option(key string) string {
	c.optionsLock.Lock()
	defer c.optionsLock.Unlock()
	return c.options[key]
}
