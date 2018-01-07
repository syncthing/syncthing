package features

import (
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
// the default setting is to have it switched off

func Test_shouldBeSwitchedOffByDefault(t *testing.T) {
	setup()
	defer cleanup()

	device1, _ := protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	defaultConfig := config.Wrap("config.xml", config.New(device1))
	options := defaultConfig.Options()

	assert.False(t, options.SingleGlobalFolderScanner, "Expected to be disabled by default")
}
