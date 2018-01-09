package features

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

// the feature 'single-global-folder-scanner'
// should make it possible to have only one active process of
// scanning and maybe hashing shared folders at a time
//
// in the global settings it can be switched on/off

func TestMain(m *testing.M) {
	tempDir := &TemporaryDirectoryForTests{}
	tempDir.Init()
	tempDir.Setup()
	defer tempDir.Cleanup()

	os.Exit(m.Run())
}

// the default setting is to have it switched off
func Test_shouldBeSwitchedOffByDefault(t *testing.T) {
	options := createDefaultConfig().Options()

	assert.False(t, options.SingleGlobalFolderScanner, "Expected to be disabled by default")
}

func createDefaultConfig() *config.Wrapper {
	cfg := config.New(protocol.LocalDeviceID)
	return createConfig(cfg)
}

func createConfig(cfg config.Configuration) *config.Wrapper {
	return config.Wrap("config.xml", cfg)
}
