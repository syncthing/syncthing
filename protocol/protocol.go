package protocol

import (
	"bufio"
	"compress/flate"
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

	cw   *countingWriter
	wb   *bufio.Writer
	xw   *xdr.Writer
	wmut sync.Mutex

	indexSent map[string]map[string][2]int64
	awaiting  []chan asyncResult
	imut      sync.Mutex

	nextID chan int
	outbox chan []encodable
	closed chan struct{}
}

type asyncResult struct {
	val []byte
	err error
}

const (
	pingTimeout  = 4 * time.Minute
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
		awaiting:  make([]chan asyncResult, 0x1000),
		indexSent: make(map[string]map[string][2]int64),
		outbox:    make(chan []encodable),
		nextID:    make(chan int),
		closed:    make(chan struct{}),
	}

	go c.readerLoop()
	go c.writerLoop()
	go c.pingerLoop()
	go c.idGenerator()

	return wireFormatConnection{&c}
}

func (c *rawConnection) ID() string {
	return c.id
}

// Index writes the list of file information to the connected peer node
func (c *rawConnection) Index(repo string, idx []FileInfo) {
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
	c.imut.Unlock()

	c.send(header{0, -1, msgType}, IndexMessage{repo, idx})
}

// Request returns the bytes for the specified block after fetching them from the connected peer.
func (c *rawConnection) Request(repo string, name string, offset int64, size int) ([]byte, error) {
	var id int
	select {
	case id = <-c.nextID:
	case <-c.closed:
		return nil, ErrClosed
	}

	c.imut.Lock()
	if ch := c.awaiting[id]; ch != nil {
		panic("id taken")
	}
	rc := make(chan asyncResult)
	c.awaiting[id] = rc
	c.imut.Unlock()

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
	c.imut.Lock()
	c.awaiting[id] = rc
	c.imut.Unlock()

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
			if err := c.handleIndex(); err != nil {
				return err
			}

		case messageTypeIndexUpdate:
			if err := c.handleIndexUpdate(); err != nil {
				return err
			}

		case messageTypeRequest:
			if err := c.handleRequest(hdr); err != nil {
				return err
			}

		case messageTypeResponse:
			if err := c.handleResponse(hdr); err != nil {
				return err
			}

		case messageTypePing:
			c.send(header{0, hdr.msgID, messageTypePong})

		case messageTypePong:
			c.handlePong(hdr)

		case messageTypeClusterConfig:
			if err := c.handleClusterConfig(); err != nil {
				return err
			}

		default:
			return fmt.Errorf("protocol error: %s: unknown message type %#x", c.id, hdr.msgType)
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

		go c.receiver.Index(c.id, im.Repository, im.Files)
	}
	return nil
}

func (c *rawConnection) handleIndexUpdate() error {
	var im IndexMessage
	im.decodeXDR(c.xr)
	if err := c.xr.Error(); err != nil {
		return err
	} else {
		go c.receiver.IndexUpdate(c.id, im.Repository, im.Files)
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

	go func(hdr header, err error) {
		c.imut.Lock()
		rc := c.awaiting[hdr.msgID]
		c.awaiting[hdr.msgID] = nil
		c.imut.Unlock()

		if rc != nil {
			rc <- asyncResult{data, err}
			close(rc)
		}
	}(hdr, c.xr.Error())

	return nil
}

func (c *rawConnection) handlePong(hdr header) {
	c.imut.Lock()
	if rc := c.awaiting[hdr.msgID]; rc != nil {
		go func() {
			rc <- asyncResult{}
			close(rc)
		}()

		c.awaiting[hdr.msgID] = nil
	}
	c.imut.Unlock()
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
	var err error
	for es := range c.outbox {
		c.wmut.Lock()
		for _, e := range es {
			e.encodeXDR(c.xw)
		}

		if err = c.flush(); err != nil {
			c.wmut.Unlock()
			c.close(err)
			return
		}
		c.wmut.Unlock()
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

	select {
	case <-c.closed:
		return
	default:
		close(c.closed)

		for i, ch := range c.awaiting {
			if ch != nil {
				close(ch)
				c.awaiting[i] = nil
			}
		}

		c.writer.Close()
		c.reader.Close()

		go c.receiver.Close(c.id, err)
	}
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

	c.send(header{0, msgID, messageTypeResponse},
		encodableBytes(data))
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
