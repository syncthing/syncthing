package scanner

import "fmt"

type File struct {
	Name       string
	Flags      uint32
	Modified   int64
	Version    uint64
	Size       int64
	Blocks     []Block
	Suppressed bool
}

func (f File) String() string {
	return fmt.Sprintf("File{Name:%q, Flags:0%o, Modified:%d, Version:%d, Size:%d, NumBlocks:%d}",
		f.Name, f.Flags, f.Modified, f.Version, f.Size, len(f.Blocks))
}

func (f File) Equals(o File) bool {
	return f.Modified == o.Modified && f.Version == o.Version
}

func (f File) NewerThan(o File) bool {
	return f.Modified > o.Modified || (f.Modified == o.Modified && f.Version > o.Version)
}
