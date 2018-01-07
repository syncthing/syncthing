package features

import "github.com/syncthing/syncthing/lib/logger"

var (
	l                          = logger.DefaultLogger.NewFacility("feature", "tests for features")
	temporaryDirectoryForTests = &TemporaryDirectoryForTests{}
)

func init() {
	temporaryDirectoryForTests.Init()
}

func setup() {
	temporaryDirectoryForTests.Setup()
}

func cleanup() {
	temporaryDirectoryForTests.Cleanup()
}
