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
	"github.com/calmh/syncthing/xdr"
)

const BlockSize = 128 * 1024

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
	FlagDeleted uint32 = 1 << 12
	FlagInvalid        = 1 << 13
)

var (
	ErrClusterHash = fmt.Errorf("configuration error: mismatched cluster hash")
	ErrClosed      = errors.New("connection closed")
)

type Model interface {
	// An index was received from the peer node
	Index(nodeID string, files []FileInfo)
	// An index update was received from the peer node
	IndexUpdate(nodeID string, files []FileInfo)
	// A request was made by the peer node
	Request(nodeID, repo string, name string, offset int64, size int) ([]byte, error)
	// The peer node closed the connection
	Close(nodeID string, err error)
}

type Connection interface {
	ID() string
	Index(string, []FileInfo)
	Request(repo, name string, offset int64, size int) ([]byte, error)
	Statistics() Statistics
	Option(key string) string
}

type rawConnection struct {
	sync.RWMutex

	id          string
	receiver    Model
	reader      io.ReadCloser
	xr          *xdr.Reader
	writer      io.WriteCloser
	xw          *xdr.Writer
	closed      chan struct{}
	awaiting    map[int]chan asyncResult
	nextID      int
	indexSent   map[string]map[string][2]int64
	peerOptions map[string]string
	myOptions   map[string]string
	optionsLock sync.Mutex

	hasSentIndex  bool
	hasRecvdIndex bool

	statisticsLock sync.Mutex
}

type asyncResult struct {
	val []byte
	err error
}

const (
	pingTimeout  = 2 * time.Minute
	pingIdleTime = 5 * time.Minute
)

func NewConnection(nodeID string, reader io.Reader, writer io.Writer, receiver Model, options map[string]string) Connection {
	flrd := flate.NewReader(reader)
	flwr, err := flate.NewWriter(writer, flate.BestSpeed)
	if err != nil {
		panic(err)
	}

	c := rawConnection{
		id:        nodeID,
		receiver:  nativeModel{receiver},
		reader:    flrd,
		xr:        xdr.NewReader(flrd),
		writer:    flwr,
		xw:        xdr.NewWriter(flwr),
		closed:    make(chan struct{}),
		awaiting:  make(map[int]chan asyncResult),
		indexSent: make(map[string]map[string][2]int64),
	}

	go c.readerLoop()
	go c.pingerLoop()

	if options != nil {
		c.myOptions = options
		go func() {
			c.Lock()
			header{0, c.nextID, messageTypeOptions}.encodeXDR(c.xw)
			var om OptionsMessage
			for k, v := range options {
				om.Options = append(om.Options, Option{k, v})
			}
			om.encodeXDR(c.xw)
			err := c.xw.Error()
			if err == nil {
				err = c.flush()
			}
			if err != nil {
				log.Println("Warning: Write error during initial handshake:", err)
			}
			c.nextID++
			c.Unlock()
		}()
	}

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
				c.receiver.Index(c.id, im.Files)
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
				c.receiver.IndexUpdate(c.id, im.Files)
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

		case messageTypeOptions:
			var om OptionsMessage
			om.decodeXDR(c.xr)
			if c.xr.Error() != nil {
				c.close(c.xr.Error())
				break loop
			}

			c.optionsLock.Lock()
			c.peerOptions = make(map[string]string, len(om.Options))
			for _, opt := range om.Options {
				c.peerOptions[opt.Key] = opt.Value
			}
			c.optionsLock.Unlock()

			if mh, rh := c.myOptions["clusterHash"], c.peerOptions["clusterHash"]; len(mh) > 0 && len(rh) > 0 && mh != rh {
				c.close(ErrClusterHash)
				break loop
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
	c.statisticsLock.Lock()
	defer c.statisticsLock.Unlock()

	stats := Statistics{
		At:            time.Now(),
		InBytesTotal:  int(c.xr.Tot()),
		OutBytesTotal: int(c.xw.Tot()),
	}

	return stats
}

func (c *rawConnection) Option(key string) string {
	c.optionsLock.Lock()
	defer c.optionsLock.Unlock()
	return c.peerOptions[key]
}
