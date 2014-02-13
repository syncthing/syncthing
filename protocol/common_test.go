package protocol

import "io"

type TestModel struct {
	data   []byte
	repo   string
	name   string
	offset int64
	size   uint32
	hash   []byte
	closed bool
}

func (t *TestModel) Index(nodeID string, files []FileInfo) {
}

func (t *TestModel) IndexUpdate(nodeID string, files []FileInfo) {
}

func (t *TestModel) Request(nodeID, repo, name string, offset int64, size uint32, hash []byte) ([]byte, error) {
	t.repo = repo
	t.name = name
	t.offset = offset
	t.size = size
	t.hash = hash
	return t.data, nil
}

func (t *TestModel) Close(nodeID string, err error) {
	t.closed = true
}

type ErrPipe struct {
	io.PipeWriter
	written int
	max     int
	err     error
	closed  bool
}

func (e *ErrPipe) Write(data []byte) (int, error) {
	if e.closed {
		return 0, e.err
	}
	if e.written+len(data) > e.max {
		n, _ := e.PipeWriter.Write(data[:e.max-e.written])
		e.PipeWriter.CloseWithError(e.err)
		e.closed = true
		return n, e.err
	} else {
		return e.PipeWriter.Write(data)
	}
}
