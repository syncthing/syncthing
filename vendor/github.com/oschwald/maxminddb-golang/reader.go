package maxminddb

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"reflect"
)

const (
	// NotFound is returned by LookupOffset when a matched root record offset
	// cannot be found.
	NotFound = ^uintptr(0)

	dataSectionSeparatorSize = 16
)

var metadataStartMarker = []byte("\xAB\xCD\xEFMaxMind.com")

// Reader holds the data corresponding to the MaxMind DB file. Its only public
// field is Metadata, which contains the metadata from the MaxMind DB file.
type Reader struct {
	hasMappedFile bool
	buffer        []byte
	decoder       decoder
	Metadata      Metadata
	ipv4Start     uint
}

// Metadata holds the metadata decoded from the MaxMind DB file. In particular
// in has the format version, the build time as Unix epoch time, the database
// type and description, the IP version supported, and a slice of the natural
// languages included.
type Metadata struct {
	BinaryFormatMajorVersion uint              `maxminddb:"binary_format_major_version"`
	BinaryFormatMinorVersion uint              `maxminddb:"binary_format_minor_version"`
	BuildEpoch               uint              `maxminddb:"build_epoch"`
	DatabaseType             string            `maxminddb:"database_type"`
	Description              map[string]string `maxminddb:"description"`
	IPVersion                uint              `maxminddb:"ip_version"`
	Languages                []string          `maxminddb:"languages"`
	NodeCount                uint              `maxminddb:"node_count"`
	RecordSize               uint              `maxminddb:"record_size"`
}

// FromBytes takes a byte slice corresponding to a MaxMind DB file and returns
// a Reader structure or an error.
func FromBytes(buffer []byte) (*Reader, error) {
	metadataStart := bytes.LastIndex(buffer, metadataStartMarker)

	if metadataStart == -1 {
		return nil, newInvalidDatabaseError("error opening database: invalid MaxMind DB file")
	}

	metadataStart += len(metadataStartMarker)
	metadataDecoder := decoder{buffer[metadataStart:]}

	var metadata Metadata

	rvMetdata := reflect.ValueOf(&metadata)
	_, err := metadataDecoder.decode(0, rvMetdata, 0)
	if err != nil {
		return nil, err
	}

	searchTreeSize := metadata.NodeCount * metadata.RecordSize / 4
	dataSectionStart := searchTreeSize + dataSectionSeparatorSize
	dataSectionEnd := uint(metadataStart - len(metadataStartMarker))
	if dataSectionStart > dataSectionEnd {
		return nil, newInvalidDatabaseError("the MaxMind DB contains invalid metadata")
	}
	d := decoder{
		buffer[searchTreeSize+dataSectionSeparatorSize : metadataStart-len(metadataStartMarker)],
	}

	reader := &Reader{
		buffer:    buffer,
		decoder:   d,
		Metadata:  metadata,
		ipv4Start: 0,
	}

	reader.ipv4Start, err = reader.startNode()

	return reader, err
}

func (r *Reader) startNode() (uint, error) {
	if r.Metadata.IPVersion != 6 {
		return 0, nil
	}

	nodeCount := r.Metadata.NodeCount

	node := uint(0)
	var err error
	for i := 0; i < 96 && node < nodeCount; i++ {
		node, err = r.readNode(node, 0)
		if err != nil {
			return 0, err
		}
	}
	return node, err
}

// Lookup takes an IP address as a net.IP structure and a pointer to the
// result value to Decode into.
func (r *Reader) Lookup(ipAddress net.IP, result interface{}) error {
	pointer, err := r.lookupPointer(ipAddress)
	if pointer == 0 || err != nil {
		return err
	}
	return r.retrieveData(pointer, result)
}

// LookupOffset maps an argument net.IP to a corresponding record offset in the
// database. NotFound is returned if no such record is found, and a record may
// otherwise be extracted by passing the returned offset to Decode. LookupOffset
// is an advanced API, which exists to provide clients with a means to cache
// previously-decoded records.
func (r *Reader) LookupOffset(ipAddress net.IP) (uintptr, error) {
	pointer, err := r.lookupPointer(ipAddress)
	if pointer == 0 || err != nil {
		return NotFound, err
	}
	return r.resolveDataPointer(pointer)
}

// Decode the record at |offset| into |result|. The result value pointed to
// must be a data value that corresponds to a record in the database. This may
// include a struct representation of the data, a map capable of holding the
// data or an empty interface{} value.
//
// If result is a pointer to a struct, the struct need not include a field
// for every value that may be in the database. If a field is not present in
// the structure, the decoder will not decode that field, reducing the time
// required to decode the record.
//
// As a special case, a struct field of type uintptr will be used to capture
// the offset of the value. Decode may later be used to extract the stored
// value from the offset. MaxMind DBs are highly normalized: for example in
// the City database, all records of the same country will reference a
// single representative record for that country. This uintptr behavior allows
// clients to leverage this normalization in their own sub-record caching.
func (r *Reader) Decode(offset uintptr, result interface{}) error {
	rv := reflect.ValueOf(result)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("result param must be a pointer")
	}

	_, err := r.decoder.decode(uint(offset), reflect.ValueOf(result), 0)
	return err
}

func (r *Reader) lookupPointer(ipAddress net.IP) (uint, error) {
	if ipAddress == nil {
		return 0, errors.New("ipAddress passed to Lookup cannot be nil")
	}

	ipV4Address := ipAddress.To4()
	if ipV4Address != nil {
		ipAddress = ipV4Address
	}
	if len(ipAddress) == 16 && r.Metadata.IPVersion == 4 {
		return 0, fmt.Errorf("error looking up '%s': you attempted to look up an IPv6 address in an IPv4-only database", ipAddress.String())
	}

	return r.findAddressInTree(ipAddress)
}

func (r *Reader) findAddressInTree(ipAddress net.IP) (uint, error) {

	bitCount := uint(len(ipAddress) * 8)

	var node uint
	if bitCount == 32 {
		node = r.ipv4Start
	}

	nodeCount := r.Metadata.NodeCount

	for i := uint(0); i < bitCount && node < nodeCount; i++ {
		bit := uint(1) & (uint(ipAddress[i>>3]) >> (7 - (i % 8)))

		var err error
		node, err = r.readNode(node, bit)
		if err != nil {
			return 0, err
		}
	}
	if node == nodeCount {
		// Record is empty
		return 0, nil
	} else if node > nodeCount {
		return node, nil
	}

	return 0, newInvalidDatabaseError("invalid node in search tree")
}

func (r *Reader) readNode(nodeNumber uint, index uint) (uint, error) {
	RecordSize := r.Metadata.RecordSize

	baseOffset := nodeNumber * RecordSize / 4

	var nodeBytes []byte
	var prefix uint
	switch RecordSize {
	case 24:
		offset := baseOffset + index*3
		nodeBytes = r.buffer[offset : offset+3]
	case 28:
		prefix = uint(r.buffer[baseOffset+3])
		if index != 0 {
			prefix &= 0x0F
		} else {
			prefix = (0xF0 & prefix) >> 4
		}
		offset := baseOffset + index*4
		nodeBytes = r.buffer[offset : offset+3]
	case 32:
		offset := baseOffset + index*4
		nodeBytes = r.buffer[offset : offset+4]
	default:
		return 0, newInvalidDatabaseError("unknown record size: %d", RecordSize)
	}
	return uintFromBytes(prefix, nodeBytes), nil
}

func (r *Reader) retrieveData(pointer uint, result interface{}) error {
	offset, err := r.resolveDataPointer(pointer)
	if err != nil {
		return err
	}
	return r.Decode(offset, result)
}

func (r *Reader) resolveDataPointer(pointer uint) (uintptr, error) {
	var resolved = uintptr(pointer - r.Metadata.NodeCount - dataSectionSeparatorSize)

	if resolved > uintptr(len(r.buffer)) {
		return 0, newInvalidDatabaseError("the MaxMind DB file's search tree is corrupt")
	}
	return resolved, nil
}
