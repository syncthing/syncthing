// +build darwin freebsd netbsd openbsd dragonfly
// +build !appengine

package pb

import "syscall"

const ioctlReadTermios = syscall.TIOCGETA
const ioctlWriteTermios = syscall.TIOCSETA
