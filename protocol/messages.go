package protocol

import (
	"errors"
	"io"

	"github.com/calmh/syncthing/buffers"
	"github.com/calmh/syncthing/xdr"
)

const (
	maxNumFiles  = 100000 // More than 100000 files is a protocol error
	maxNumBlocks = 100000 // 100000 * 128KB = 12.5 GB max acceptable file size
)

var (
	ErrMaxFilesExceeded  = errors.New("Protocol error: number of files per index exceeds limit")
	ErrMaxBlocksExceeded = errors.New("Protocol error: number of blocks per file exceeds limit")
)

type request struct {
	repo   string
	name   string
	offset int64
	size   uint32
	hash   []byte
}

type header struct {
	version int
	msgID   int
	msgType int
}

func encodeHeader(h header) uint32 {
	return uint32(h.version&0xf)<<28 +
		uint32(h.msgID&0xfff)<<16 +
		uint32(h.msgType&0xff)<<8
}

func decodeHeader(u uint32) header {
	return header{
		version: int(u>>28) & 0xf,
		msgID:   int(u>>16) & 0xfff,
		msgType: int(u>>8) & 0xff,
	}
}

func WriteIndex(w io.Writer, repo string, idx []FileInfo) (int, error) {
	mw := newMarshalWriter(w)
	mw.writeIndex(repo, idx)
	return int(mw.Tot()), mw.Err()
}

type marshalWriter struct {
	*xdr.Writer
}

func newMarshalWriter(w io.Writer) marshalWriter {
	return marshalWriter{xdr.NewWriter(w)}
}

func (w *marshalWriter) writeHeader(h header) {
	w.WriteUint32(encodeHeader(h))
}

func (w *marshalWriter) writeIndex(repo string, idx []FileInfo) {
	w.WriteString(repo)
	w.WriteUint32(uint32(len(idx)))
	for _, f := range idx {
		w.WriteString(f.Name)
		w.WriteUint32(f.Flags)
		w.WriteUint64(uint64(f.Modified))
		w.WriteUint32(f.Version)
		w.WriteUint32(uint32(len(f.Blocks)))
		for _, b := range f.Blocks {
			w.WriteUint32(b.Size)
			w.WriteBytes(b.Hash)
		}
	}
}

func (w *marshalWriter) writeRequest(r request) {
	w.WriteString(r.repo)
	w.WriteString(r.name)
	w.WriteUint64(uint64(r.offset))
	w.WriteUint32(r.size)
	w.WriteBytes(r.hash)
}

func (w *marshalWriter) writeResponse(data []byte) {
	w.WriteBytes(data)
}

func (w *marshalWriter) writeOptions(opts map[string]string) {
	w.WriteUint32(uint32(len(opts)))
	for k, v := range opts {
		w.WriteString(k)
		w.WriteString(v)
	}
}

func ReadIndex(r io.Reader) (string, []FileInfo, error) {
	mr := newMarshalReader(r)
	repo, idx := mr.readIndex()
	return repo, idx, mr.Err()
}

type marshalReader struct {
	*xdr.Reader
	err error
}

func newMarshalReader(r io.Reader) marshalReader {
	return marshalReader{
		Reader: xdr.NewReader(r),
		err:    nil,
	}
}

func (r marshalReader) Err() error {
	if r.err != nil {
		return r.err
	}
	return r.Reader.Err()
}

func (r marshalReader) readHeader() header {
	return decodeHeader(r.ReadUint32())
}

func (r marshalReader) readIndex() (string, []FileInfo) {
	var files []FileInfo
	repo := r.ReadString()
	nfiles := r.ReadUint32()
	if nfiles > maxNumFiles {
		r.err = ErrMaxFilesExceeded
		return "", nil
	}
	if nfiles > 0 {
		files = make([]FileInfo, nfiles)
		for i := range files {
			files[i].Name = r.ReadString()
			files[i].Flags = r.ReadUint32()
			files[i].Modified = int64(r.ReadUint64())
			files[i].Version = r.ReadUint32()
			nblocks := r.ReadUint32()
			if nblocks > maxNumBlocks {
				r.err = ErrMaxBlocksExceeded
				return "", nil
			}
			blocks := make([]BlockInfo, nblocks)
			for j := range blocks {
				blocks[j].Size = r.ReadUint32()
				blocks[j].Hash = r.ReadBytes(buffers.Get(32))
			}
			files[i].Blocks = blocks
		}
	}
	return repo, files
}

func (r marshalReader) readRequest() request {
	var req request
	req.repo = r.ReadString()
	req.name = r.ReadString()
	req.offset = int64(r.ReadUint64())
	req.size = r.ReadUint32()
	req.hash = r.ReadBytes(buffers.Get(32))
	return req
}

func (r marshalReader) readResponse() []byte {
	return r.ReadBytes(buffers.Get(128 * 1024))
}

func (r marshalReader) readOptions() map[string]string {
	n := r.ReadUint32()
	opts := make(map[string]string, n)
	for i := 0; i < int(n); i++ {
		k := r.ReadString()
		v := r.ReadString()
		opts[k] = v
	}
	return opts
}
