package discover

const (
	AnnouncementMagicV1 = 0x20121025
	QueryMagicV1        = 0x19760309
)

type QueryV1 struct {
	Magic  uint32
	NodeID string // max:64
}

type AnnounceV1 struct {
	Magic  uint32
	Port   uint16
	NodeID string // max:64
	IP     []byte // max:16
}

const (
	AnnouncementMagicV2 = 0x029E4C77
	QueryMagicV2        = 0x23D63A9A
)

type QueryV2 struct {
	Magic  uint32
	NodeID string // max:64
}

type AnnounceV2 struct {
	Magic     uint32
	NodeID    string    // max:64
	Addresses []Address // max:16
}

type Address struct {
	IP   []byte // max:16
	Port uint16
}
