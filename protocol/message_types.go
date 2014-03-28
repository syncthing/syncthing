package protocol

type IndexMessage struct {
	Repository string     // max:64
	Files      []FileInfo // max:100000
}

type FileInfo struct {
	Name     string // max:1024
	Flags    uint32
	Modified int64
	Version  uint64
	Blocks   []BlockInfo // max:100000
}

type BlockInfo struct {
	Size uint32
	Hash []byte // max:64
}

type RequestMessage struct {
	Repository string // max:64
	Name       string // max:1024
	Offset     uint64
	Size       uint32
}

type OptionsMessage struct {
	Options []Option // max:64
}

type Option struct {
	Key   string // max:64
	Value string // max:1024
}
