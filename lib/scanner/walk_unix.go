//go:build !windows
// +build !windows

package scanner

import (
	"syscall"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

func setSyscallStatData(fi *protocol.FileInfo, stat fs.FileInfo) {
	if sys, ok := stat.Sys().(*syscall.Stat_t); ok {
		fi.InodeChangeNs = sys.Ctimespec.Nano()
	}
}
