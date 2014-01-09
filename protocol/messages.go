package protocol

import "io"

type request struct {
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

func (w *marshalWriter) writeHeader(h header) {
	w.writeUint32(encodeHeader(h))
}

func (w *marshalWriter) writeIndex(idx []FileInfo) {
	w.writeUint32(uint32(len(idx)))
	for _, f := range idx {
		w.writeString(f.Name)
		w.writeUint32(f.Flags)
		w.writeUint64(uint64(f.Modified))
		w.writeUint32(f.Version)
		w.writeUint32(uint32(len(f.Blocks)))
		for _, b := range f.Blocks {
			w.writeUint32(b.Size)
			w.writeBytes(b.Hash)
		}
	}
}

func WriteIndex(w io.Writer, idx []FileInfo) (int, error) {
	mw := marshalWriter{w: w}
	mw.writeIndex(idx)
	return int(mw.getTot()), mw.err
}

func (w *marshalWriter) writeRequest(r request) {
	w.writeString(r.name)
	w.writeUint64(uint64(r.offset))
	w.writeUint32(r.size)
	w.writeBytes(r.hash)
}

func (w *marshalWriter) writeResponse(data []byte) {
	w.writeBytes(data)
}

func (r *marshalReader) readHeader() header {
	return decodeHeader(r.readUint32())
}

func (r *marshalReader) readIndex() []FileInfo {
	var files []FileInfo
	nfiles := r.readUint32()
	if nfiles > 0 {
		files = make([]FileInfo, nfiles)
		for i := range files {
			files[i].Name = r.readString()
			files[i].Flags = r.readUint32()
			files[i].Modified = int64(r.readUint64())
			files[i].Version = r.readUint32()
			nblocks := r.readUint32()
			blocks := make([]BlockInfo, nblocks)
			for j := range blocks {
				blocks[j].Size = r.readUint32()
				blocks[j].Hash = r.readBytes()
			}
			files[i].Blocks = blocks
		}
	}
	return files
}

func ReadIndex(r io.Reader) ([]FileInfo, error) {
	mr := marshalReader{r: r}
	idx := mr.readIndex()
	return idx, mr.err
}

func (r *marshalReader) readRequest() request {
	var req request
	req.name = r.readString()
	req.offset = int64(r.readUint64())
	req.size = r.readUint32()
	req.hash = r.readBytes()
	return req
}

func (r *marshalReader) readResponse() []byte {
	return r.readBytes()
}
