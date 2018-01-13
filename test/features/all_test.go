package features

import (
	"os"
	"testing"

	"github.com/syncthing/syncthing/test"
)

func TestMain(m *testing.M) {
	tempDir := test.NewTemporaryDirectoryForTests()
	defer tempDir.Cleanup()

	os.Exit(m.Run())
}
