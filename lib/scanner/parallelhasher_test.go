package scanner

import (
	"context"
	"os"
	gosync "sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/test"
)

func TestMain(m *testing.M) {
	tempDir := test.NewTemporaryDirectoryForTests()
	defer tempDir.Cleanup()

	os.Exit(m.Run())
}

var (
	hConfig = &hashConfig{
		filesystem: fs.NewFilesystem(fs.FilesystemTypeBasic, "."),
		blockSize:  16,
	}
	outbox = make(chan<- protocol.FileInfo)
	inbox  = make(<-chan protocol.FileInfo)
)

func Test_should(t *testing.T) {

	outbox := make(chan<- protocol.FileInfo)
	inbox := make(<-chan protocol.FileInfo)
	newParallelHasher(hConfig, 100, outbox, inbox, nil).run(context.TODO(), &noGlobalFolderScannerLimiter{})
	//assert.False(t, options.SingleGlobalFolderScanner, "Expected to be disabled by default")
}

func Test_shouldCallExactNumberOfWorkers(t *testing.T) {
	h := newParallelHasher(hConfig, 100, outbox, inbox, make(chan struct{}))

	countedWaitGroup := &countedWaitGroup{}
	h.wg = countedWaitGroup

	h.run(context.TODO(), &noGlobalFolderScannerLimiter{})

	assert.Equal(t, 100, countedWaitGroup.count)
}

type countedWaitGroup struct {
	gosync.WaitGroup
	count int
}

func (wg *countedWaitGroup) Add(delta int) {
	wg.WaitGroup.Add(delta)
	wg.count++
}
