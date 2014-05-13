package protocol

type IndexMessage struct {
	Repository string     // max:64
	Files      []FileInfo // max:1000000
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

type ClusterConfigMessage struct {
	ClientName    string       // max:64
	ClientVersion string       // max:64
	Repositories  []Repository // max:64
	Options       []Option     // max:64
}

type Repository struct {
	ID    string // max:64
	Nodes []Node // max:64
}

type Node struct {
	ID    string // max:64
	Flags uint32
}

type Option struct {
	Key   string // max:64
	Value string // max:1024
}
