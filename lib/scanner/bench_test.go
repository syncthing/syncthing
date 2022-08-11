package scanner

import (
	"fmt"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
)

func BenchmarkWalk(b *testing.B) {
	testFs := fs.NewFilesystem(fs.FilesystemTypeBasic, b.TempDir())

	for i := 0; i < 100; i++ {
		if err := testFs.Mkdir(fmt.Sprintf("dir%d", i), 0755); err != nil {
			b.Fatal(err)
		}
		for j := 0; j < 100; j++ {
			if fd, err := testFs.Create(fmt.Sprintf("dir%d/file%d", i, j)); err != nil {
				b.Fatal(err)
			} else {
				fd.Close()
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		walkDir(testFs, "/", nil, nil, 0)
	}
}
