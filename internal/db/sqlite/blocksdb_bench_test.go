//go:build slow

package sqlite

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

func TestBenchmarkLocalInsert(t *testing.T) {
	st, _ := strconv.Atoi(os.Getenv("SHARDING_THRESHOLD"))
	db, err := Open(t.TempDir(), WithShardingThreshold(st))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	const numFiles = 1000
	const numBlocks = 1567

	fs := make([]protocol.FileInfo, numFiles)
	t0 := time.Now()
	var totFiles, totBlocks int

	fmt.Println("TIME,FILES,BLOCKS,FILES/S,BLOCKS/S")
	for totBlocks < 200_000_000 { // ~ 24 TiB at minimum block size
		for i := range fs {
			fs[i] = genFile(rand.String(24), numBlocks, 0)
		}

		t1 := time.Now()

		if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
			t.Fatal(err)
		}

		insFiles := numFiles              // curFiles - totFiles
		insBlocks := numFiles * numBlocks // curBlocks - totBlocks
		totFiles += insFiles
		totBlocks += insBlocks

		d0 := time.Since(t0)
		d1 := time.Since(t1)

		fmt.Printf("%.2f,%d,%d,%.01f,%.01f\n", d0.Seconds(), totFiles, totBlocks, float64(insFiles)/d1.Seconds(), float64(insBlocks)/d1.Seconds())
	}
}
