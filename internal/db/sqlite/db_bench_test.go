package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

func BenchmarkUpdate(b *testing.B) {
	db, err := OpenMemory()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
	})

	fs := make([]protocol.FileInfo, 100)
	seed := 0

	size := 10000
	for size < 200_000 {
		t0 := time.Now()
		if err := db.periodic(context.Background()); err != nil {
			b.Fatal(err)
		}
		b.Log("garbage collect in", time.Since(t0))

		for {
			local, err := db.LocalSize(folderID, protocol.LocalDeviceID)
			if err != nil {
				b.Fatal(err)
			}
			if local.Files >= size {
				break
			}
			fs := make([]protocol.FileInfo, 1000)
			for i := range fs {
				fs[i] = genFile(rand.String(24), 64, 0)
			}
			if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
				b.Fatal(err)
			}
		}

		b.Run(fmt.Sprintf("Insert100@%d", size), func(b *testing.B) {
			for range b.N {
				for i := range fs {
					fs[i] = genFile(rand.String(24), 64, 0)
				}
				if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N)*100.0/b.Elapsed().Seconds(), "files/s")
		})

		b.Run(fmt.Sprintf("RepBlocks100@%d", size), func(b *testing.B) {
			for range b.N {
				for i := range fs {
					fs[i].Blocks = genBlocks(fs[i].Name, seed, 64)
					fs[i].Version = fs[i].Version.Update(42)
				}
				seed++
				if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N)*100.0/b.Elapsed().Seconds(), "files/s")
		})

		b.Run(fmt.Sprintf("RepSame100@%d", size), func(b *testing.B) {
			for range b.N {
				for i := range fs {
					fs[i].Version = fs[i].Version.Update(42)
				}
				if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N)*100.0/b.Elapsed().Seconds(), "files/s")
		})

		size <<= 1
	}
}
