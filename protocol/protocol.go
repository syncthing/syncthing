package protocol

import (
	"bufio"
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/calmh/syncthing/buffers"
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
	FlagDeleted   uint32 = 1 << 12
	FlagInvalid          = 1 << 13
	FlagDirectory        = 1 << 14
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
	Index(nodeID string, repo string, files []FileInfo)
	// An index update was received from the peer node
	IndexUpdate(nodeID string, repo string, files []FileInfo)
	// A request was made by the peer node
	Request(nodeID string, repo string, name string, offset int64, size int) ([]byte, error)
	// A cluster configuration message was received
	ClusterConfig(nodeID string, config ClusterConfigMessage)
	// The peer node closed the connection
	Close(nodeID string, err error)
}

type Connection interface {
	ID() string
	Index(repo string, files []FileInfo)
	Request(repo string, name string, offset int64, size int) ([]byte, error)
	ClusterConfig(config ClusterConfigMessage)
	Statistics() Statistics
}

type rawConnection struct {
	id       string
	receiver Model

	reader io.ReadCloser
	cr     *countingReader
	xr     *xdr.Reader
	writer io.WriteCloser

	cw     *countingWriter
	wb     *bufio.Writer
	xw     *xdr.Writer
	wmut   sync.Mutex
	closed bool

	awaiting  map[int]chan asyncResult
	nextID    int
	indexSent map[string]map[string][2]int64
	imut      sync.Mutex
}

type asyncResult struct {
	val []byte
	err error
}

const (
	pingTimeout  = 2 * time.Minute
	pingIdleTime = 5 * time.Minute
)

func NewConnection(nodeID string, reader io.Reader, writer io.Writer, receiver Model) Connection {
	cr := &countingReader{Reader: reader}
	cw := &countingWriter{Writer: writer}

	flrd := flate.NewReader(cr)
	flwr, err := flate.NewWriter(cw, flate.BestSpeed)
	if err != nil {
		panic(err)
	}
	wb := bufio.NewWriter(flwr)

	c := rawConnection{
		id:        nodeID,
		receiver:  nativeModel{receiver},
		reader:    flrd,
		cr:        cr,
		xr:        xdr.NewReader(flrd),
		writer:    flwr,
		cw:        cw,
		wb:        wb,
		xw:        xdr.NewWriter(wb),
		awaiting:  make(map[int]chan asyncResult),
		indexSent: make(map[string]map[string][2]int64),
	}

	go c.readerLoop()
	go c.pingerLoop()

	return wireFormatConnection{&c}
}

func (c *rawConnection) ID() string {
	return c.id
}

// Index writes the list of file information to the connected peer node
func (c *rawConnection) Index(repo string, idx []FileInfo) {
	if c.isClosed() {
		return
	}

	c.imut.Lock()
	var msgType int
	if c.indexSent[repo] == nil {
		// This is the first time we send an index.
		msgType = messageTypeIndex

		c.indexSent[repo] = make(map[string][2]int64)
		for _, f := range idx {
			c.indexSent[repo][f.Name] = [2]int64{f.Modified, int64(f.Version)}
		}
	} else {
		// We have sent one full index. Only send updates now.
		msgType = messageTypeIndexUpdate
		var diff []FileInfo
		for _, f := range idx {
			if vs, ok := c.indexSent[repo][f.Name]; !ok || f.Modified != vs[0] || int64(f.Version) != vs[1] {
				diff = append(diff, f)
				c.indexSent[repo][f.Name] = [2]int64{f.Modified, int64(f.Version)}
			}
		}
		idx = diff
	}

	id := c.nextID
	c.nextID = (c.nextID + 1) & 0xfff
	c.imut.Unlock()

	c.wmut.Lock()
	header{0, id, msgType}.encodeXDR(c.xw)
	IndexMessage{repo, idx}.encodeXDR(c.xw)
	err := c.flush()
	c.wmut.Unlock()

	if err != nil {
		c.close(err)
		return
	}
}

// Request returns the bytes for the specified block after fetching them from the connected peer.
func (c *rawConnection) Request(repo string, name string, offset int64, size int) ([]byte, error) {
	if c.isClosed() {
		return nil, ErrClosed
	}

	c.imut.Lock()
	id := c.nextID
	c.nextID = (c.nextID + 1) & 0xfff
	rc := make(chan asyncResult)
	if _, ok := c.awaiting[id]; ok {
		panic("id taken")
	}
	c.awaiting[id] = rc
	c.imut.Unlock()

	c.wmut.Lock()
	header{0, id, messageTypeRequest}.encodeXDR(c.xw)
	RequestMessage{repo, name, uint64(offset), uint32(size)}.encodeXDR(c.xw)
	err := c.flush()
	c.wmut.Unlock()

	if err != nil {
		c.close(err)
		return nil, err
	}

	res, ok := <-rc
	if !ok {
		return nil, ErrClosed
	}
	return res.val, res.err
}

// ClusterConfig send the cluster configuration message to the peer and returns any error
func (c *rawConnection) ClusterConfig(config ClusterConfigMessage) {
	if c.isClosed() {
		return
	}

	c.imut.Lock()
	id := c.nextID
	c.nextID = (c.nextID + 1) & 0xfff
	c.imut.Unlock()

	c.wmut.Lock()
	header{0, id, messageTypeClusterConfig}.encodeXDR(c.xw)
	config.encodeXDR(c.xw)
	err := c.flush()
	c.wmut.Unlock()

	if err != nil {
		c.close(err)
	}
}

func (c *rawConnection) ping() bool {
	if c.isClosed() {
		return false
	}

	c.imut.Lock()
	id := c.nextID
	c.nextID = (c.nextID + 1) & 0xfff
	rc := make(chan asyncResult, 1)
	c.awaiting[id] = rc
	c.imut.Unlock()

	c.wmut.Lock()
	header{0, id, messageTypePing}.encodeXDR(c.xw)
	err := c.flush()
	c.wmut.Unlock()

	if err != nil {
		c.close(err)
		return false
	}

	res, ok := <-rc
	return ok && res.err == nil
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

	if f, ok := c.writer.(flusher); ok {
		return f.Flush()
	}

	return nil
}

func (c *rawConnection) close(err error) {
	c.imut.Lock()
	c.wmut.Lock()
	defer c.imut.Unlock()
	defer c.wmut.Unlock()

	if c.closed {
		return
	}

	c.closed = true

	for _, ch := range c.awaiting {
		close(ch)
	}
	c.awaiting = nil
	c.writer.Close()
	c.reader.Close()

	c.receiver.Close(c.id, err)
}

func (c *rawConnection) isClosed() bool {
	c.wmut.Lock()
	defer c.wmut.Unlock()
	return c.closed
}

func (c *rawConnection) readerLoop() {
loop:
	for !c.isClosed() {
		var hdr header
		hdr.decodeXDR(c.xr)
		if err := c.xr.Error(); err != nil {
			c.close(err)
			break loop
		}
		if hdr.version != 0 {
			c.close(fmt.Errorf("protocol error: %s: unknown message version %#x", c.id, hdr.version))
			break loop
		}

		switch hdr.msgType {
		case messageTypeIndex:
			var im IndexMessage
			im.decodeXDR(c.xr)
			if err := c.xr.Error(); err != nil {
				c.close(err)
				break loop
			} else {

				// We run this (and the corresponding one for update, below)
				// in a separate goroutine to avoid blocking the read loop.
				// There is otherwise a potential deadlock where both sides
				// has the model locked because it's sending a large index
				// update and can't receive the large index update from the
				// other side.

				go c.receiver.Index(c.id, im.Repository, im.Files)
			}

		case messageTypeIndexUpdate:
			var im IndexMessage
			im.decodeXDR(c.xr)
			if err := c.xr.Error(); err != nil {
				c.close(err)
				break loop
			} else {
				go c.receiver.IndexUpdate(c.id, im.Repository, im.Files)
			}

		case messageTypeRequest:
			var req RequestMessage
			req.decodeXDR(c.xr)
			if err := c.xr.Error(); err != nil {
				c.close(err)
				break loop
			}
			go c.processRequest(hdr.msgID, req)

		case messageTypeResponse:
			data := c.xr.ReadBytesMax(256 * 1024) // Sufficiently larger than max expected block size

			if err := c.xr.Error(); err != nil {
				c.close(err)
				break loop
			}

			go func(hdr header, err error) {
				c.imut.Lock()
				rc, ok := c.awaiting[hdr.msgID]
				delete(c.awaiting, hdr.msgID)
				c.imut.Unlock()

				if ok {
					rc <- asyncResult{data, err}
					close(rc)
				}
			}(hdr, c.xr.Error())

		case messageTypePing:
			c.wmut.Lock()
			header{0, hdr.msgID, messageTypePong}.encodeXDR(c.xw)
			err := c.flush()
			c.wmut.Unlock()
			if err != nil {
				c.close(err)
				break loop
			}

		case messageTypePong:
			c.imut.Lock()
			rc, ok := c.awaiting[hdr.msgID]

			if ok {
				go func() {
					rc <- asyncResult{}
					close(rc)
				}()

				delete(c.awaiting, hdr.msgID)
			}
			c.imut.Unlock()

		case messageTypeClusterConfig:
			var cm ClusterConfigMessage
			cm.decodeXDR(c.xr)
			if err := c.xr.Error(); err != nil {
				c.close(err)
				break loop
			} else {
				go c.receiver.ClusterConfig(c.id, cm)
			}

		default:
			c.close(fmt.Errorf("protocol error: %s: unknown message type %#x", c.id, hdr.msgType))
			break loop
		}
	}
}

func (c *rawConnection) processRequest(msgID int, req RequestMessage) {
	data, _ := c.receiver.Request(c.id, req.Repository, req.Name, int64(req.Offset), int(req.Size))

	c.wmut.Lock()
	header{0, msgID, messageTypeResponse}.encodeXDR(c.xw)
	c.xw.WriteBytes(data)
	err := c.flush()
	c.wmut.Unlock()

	buffers.Put(data)

	if err != nil {
		c.close(err)
	}
}

func (c *rawConnection) pingerLoop() {
	var rc = make(chan bool, 1)
	ticker := time.Tick(pingIdleTime / 2)
	for {
		if c.isClosed() {
			return
		}
		select {
		case <-ticker:
			go func() {
				rc <- c.ping()
			}()
			select {
			case ok := <-rc:
				if !ok {
					c.close(fmt.Errorf("ping failure"))
				}
			case <-time.After(pingTimeout):
				c.close(fmt.Errorf("ping timeout"))
			}
		}
	}
}

type Statistics struct {
	At            time.Time
	InBytesTotal  int
	OutBytesTotal int
}

func (c *rawConnection) Statistics() Statistics {
	return Statistics{
		At:            time.Now(),
		InBytesTotal:  int(c.cr.Tot()),
		OutBytesTotal: int(c.cw.Tot()),
	}
}
