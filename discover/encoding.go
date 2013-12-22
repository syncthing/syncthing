package discover

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type packet struct {
	magic uint32 // AnnouncementMagic or QueryMagic
	port  uint16 // unset if magic == QueryMagic
	id    string
	ip    []byte // zero length in local announcements
}

var (
	errBadMagic = errors.New("bad magic")
	errFormat   = errors.New("incorrect packet format")
)

func encodePacket(pkt packet) []byte {
	if l := len(pkt.ip); l != 0 && l != 4 && l != 16 {
		// bad ip format
		return nil
	}

	var idbs = []byte(pkt.id)
	var l = 4 + 4 + len(idbs) + pad(len(idbs))
	if pkt.magic == AnnouncementMagic {
		l += 4 + 4 + len(pkt.ip)
	}

	var buf = make([]byte, l)
	var offset = 0

	binary.BigEndian.PutUint32(buf[offset:], pkt.magic)
	offset += 4

	if pkt.magic == AnnouncementMagic {
		binary.BigEndian.PutUint16(buf[offset:], uint16(pkt.port))
		offset += 4
	}

	binary.BigEndian.PutUint32(buf[offset:], uint32(len(idbs)))
	offset += 4
	copy(buf[offset:], idbs)
	offset += len(idbs) + pad(len(idbs))

	if pkt.magic == AnnouncementMagic {
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(pkt.ip)))
		offset += 4
		copy(buf[offset:], pkt.ip)
		offset += len(pkt.ip)
	}

	return buf
}

func decodePacket(buf []byte) (*packet, error) {
	var p packet
	var offset int

	if len(buf) < 4 {
		// short packet
		return nil, errFormat
	}
	p.magic = binary.BigEndian.Uint32(buf[offset:])
	offset += 4

	if p.magic != AnnouncementMagic && p.magic != QueryMagic {
		return nil, errBadMagic
	}

	if p.magic == AnnouncementMagic {
		// Port Number

		if len(buf) < offset+4 {
			// short packet
			return nil, errFormat
		}
		p.port = binary.BigEndian.Uint16(buf[offset:])
		offset += 2
		reserved := binary.BigEndian.Uint16(buf[offset:])
		if reserved != 0 {
			return nil, errFormat
		}
		offset += 2
	}

	// Node ID

	if len(buf) < offset+4 {
		// short packet
		return nil, errFormat
	}
	l := binary.BigEndian.Uint32(buf[offset:])
	offset += 4

	if len(buf) < offset+int(l)+pad(int(l)) {
		// short packet
		return nil, errFormat
	}
	idbs := buf[offset : offset+int(l)]
	p.id = string(idbs)
	offset += int(l) + pad(int(l))

	if p.magic == AnnouncementMagic {
		// IP

		if len(buf) < offset+4 {
			// short packet
			return nil, errFormat
		}
		l = binary.BigEndian.Uint32(buf[offset:])
		offset += 4

		if l != 0 && l != 4 && l != 16 {
			// weird ip length
			return nil, errFormat
		}
		if len(buf) < offset+int(l) {
			// short packet
			return nil, errFormat
		}
		if l > 0 {
			p.ip = buf[offset : offset+int(l)]
			offset += int(l)
		}
	}

	if len(buf[offset:]) > 0 {
		// extra data
		return nil, errFormat
	}

	return &p, nil
}

func pad(l int) int {
	d := l % 4
	if d == 0 {
		return 0
	}
	return 4 - d
}

func ipStr(ip []byte) string {
	switch len(ip) {
	case 4:
		return fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3])
	case 16:
		return fmt.Sprintf("%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x",
			ip[0], ip[1], ip[2], ip[3],
			ip[4], ip[5], ip[6], ip[7],
			ip[8], ip[9], ip[10], ip[11],
			ip[12], ip[13], ip[14], ip[15])
	default:
		return ""
	}
}
