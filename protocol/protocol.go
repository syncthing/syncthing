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
	sync.RWMutex

	id        string
	receiver  Model
	reader    io.ReadCloser
	cr        *countingReader
	xr        *xdr.Reader
	writer    io.WriteCloser
	cw        *countingWriter
	wb        *bufio.Writer
	xw        *xdr.Writer
	closed    chan struct{}
	awaiting  map[int]chan asyncResult
	nextID    int
	indexSent map[string]map[string][2]int64

	hasSentIndex  bool
	hasRecvdIndex bool
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
		closed:    make(chan struct{}),
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
	c.Lock()
	if c.isClosed() {
		c.Unlock()
		return
	}
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

	header{0, c.nextID, msgType}.encodeXDR(c.xw)
	_, err := IndexMessage{repo, idx}.encodeXDR(c.xw)
	if err == nil {
		err = c.flush()
	}
	c.nextID = (c.nextID + 1) & 0xfff
	c.hasSentIndex = true
	c.Unlock()

	if err != nil {
		c.close(err)
		return
	}
}

// Request returns the bytes for the specified block after fetching them from the connected peer.
func (c *rawConnection) Request(repo string, name string, offset int64, size int) ([]byte, error) {
	c.Lock()
	if c.isClosed() {
		c.Unlock()
		return nil, ErrClosed
	}
	rc := make(chan asyncResult)
	if _, ok := c.awaiting[c.nextID]; ok {
		panic("id taken")
	}
	c.awaiting[c.nextID] = rc
	header{0, c.nextID, messageTypeRequest}.encodeXDR(c.xw)
	_, err := RequestMessage{repo, name, uint64(offset), uint32(size)}.encodeXDR(c.xw)
	if err == nil {
		err = c.flush()
	}
	if err != nil {
		c.Unlock()
		c.close(err)
		return nil, err
	}
	c.nextID = (c.nextID + 1) & 0xfff
	c.Unlock()

	res, ok := <-rc
	if !ok {
		return nil, ErrClosed
	}
	return res.val, res.err
}

// ClusterConfig send the cluster configuration message to the peer and returns any error
func (c *rawConnection) ClusterConfig(config ClusterConfigMessage) {
	c.Lock()
	defer c.Unlock()

	if c.isClosed() {
		return
	}

	header{0, c.nextID, messageTypeClusterConfig}.encodeXDR(c.xw)
	c.nextID = (c.nextID + 1) & 0xfff

	_, err := config.encodeXDR(c.xw)
	if err == nil {
		err = c.flush()
	}
	if err != nil {
		c.close(err)
	}
}

func (c *rawConnection) ping() bool {
	c.Lock()
	if c.isClosed() {
		c.Unlock()
		return false
	}
	rc := make(chan asyncResult, 1)
	c.awaiting[c.nextID] = rc
	header{0, c.nextID, messageTypePing}.encodeXDR(c.xw)
	err := c.flush()
	if err != nil {
		c.Unlock()
		c.close(err)
		return false
	} else if c.xw.Error() != nil {
		c.Unlock()
		c.close(c.xw.Error())
		return false
	}
	c.nextID = (c.nextID + 1) & 0xfff
	c.Unlock()

	res, ok := <-rc
	return ok && res.err == nil
}

type flusher interface {
	Flush() error
}

func (c *rawConnection) flush() error {
	c.wb.Flush()
	if f, ok := c.writer.(flusher); ok {
		return f.Flush()
	}
	return nil
}

func (c *rawConnection) close(err error) {
	c.Lock()
	select {
	case <-c.closed:
		c.Unlock()
		return
	default:
	}
	close(c.closed)
	for _, ch := range c.awaiting {
		close(ch)
	}
	c.awaiting = nil
	c.writer.Close()
	c.reader.Close()
	c.Unlock()

	c.receiver.Close(c.id, err)
}

func (c *rawConnection) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *rawConnection) readerLoop() {
loop:
	for !c.isClosed() {
		var hdr header
		hdr.decodeXDR(c.xr)
		if c.xr.Error() != nil {
			c.close(c.xr.Error())
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
			if c.xr.Error() != nil {
				c.close(c.xr.Error())
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
			c.Lock()
			c.hasRecvdIndex = true
			c.Unlock()

		case messageTypeIndexUpdate:
			var im IndexMessage
			im.decodeXDR(c.xr)
			if c.xr.Error() != nil {
				c.close(c.xr.Error())
				break loop
			} else {
				go c.receiver.IndexUpdate(c.id, im.Repository, im.Files)
			}

		case messageTypeRequest:
			var req RequestMessage
			req.decodeXDR(c.xr)
			if c.xr.Error() != nil {
				c.close(c.xr.Error())
				break loop
			}
			go c.processRequest(hdr.msgID, req)

		case messageTypeResponse:
			data := c.xr.ReadBytesMax(256 * 1024) // Sufficiently larger than max expected block size

			if c.xr.Error() != nil {
				c.close(c.xr.Error())
				break loop
			}

			go func(hdr header, err error) {
				c.Lock()
				rc, ok := c.awaiting[hdr.msgID]
				delete(c.awaiting, hdr.msgID)
				c.Unlock()

				if ok {
					rc <- asyncResult{data, err}
					close(rc)
				}
			}(hdr, c.xr.Error())

		case messageTypePing:
			c.Lock()
			header{0, hdr.msgID, messageTypePong}.encodeXDR(c.xw)
			err := c.flush()
			c.Unlock()
			if err != nil {
				c.close(err)
				break loop
			} else if c.xw.Error() != nil {
				c.close(c.xw.Error())
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

		case messageTypeClusterConfig:
			var cm ClusterConfigMessage
			cm.decodeXDR(c.xr)
			if c.xr.Error() != nil {
				c.close(c.xr.Error())
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

	c.Lock()
	header{0, msgID, messageTypeResponse}.encodeXDR(c.xw)
	_, err := c.xw.WriteBytes(data)
	if err == nil {
		err = c.flush()
	}
	c.Unlock()

	buffers.Put(data)
	if err != nil {
		c.close(err)
	}
}

func (c *rawConnection) pingerLoop() {
	var rc = make(chan bool, 1)
	ticker := time.Tick(pingIdleTime / 2)
	for {
		select {
		case <-ticker:
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
						c.close(fmt.Errorf("ping failure"))
					}
				case <-time.After(pingTimeout):
					c.close(fmt.Errorf("ping timeout"))
				}
			}
		case <-c.closed:
			return
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
