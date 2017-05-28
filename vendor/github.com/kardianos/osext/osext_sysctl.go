// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !go1.8,darwin !go1.8,freebsd openbsd

package osext

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

var initCwd, initCwdErr = os.Getwd()

func executable() (string, error) {
	var mib [4]int32
	switch runtime.GOOS {
	case "freebsd":
		mib = [4]int32{1 /* CTL_KERN */, 14 /* KERN_PROC */, 12 /* KERN_PROC_PATHNAME */, -1}
	case "darwin":
		mib = [4]int32{1 /* CTL_KERN */, 38 /* KERN_PROCARGS */, int32(os.Getpid()), -1}
	case "openbsd":
		mib = [4]int32{1 /* CTL_KERN */, 55 /* KERN_PROC_ARGS */, int32(os.Getpid()), 1 /* KERN_PROC_ARGV */}
	}

	n := uintptr(0)
	// Get length.
	_, _, errNum := syscall.Syscall6(syscall.SYS___SYSCTL, uintptr(unsafe.Pointer(&mib[0])), 4, 0, uintptr(unsafe.Pointer(&n)), 0, 0)
	if errNum != 0 {
		return "", errNum
	}
	if n == 0 { // This shouldn't happen.
		return "", nil
	}
	buf := make([]byte, n)
	_, _, errNum = syscall.Syscall6(syscall.SYS___SYSCTL, uintptr(unsafe.Pointer(&mib[0])), 4, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)), 0, 0)
	if errNum != 0 {
		return "", errNum
	}
	if n == 0 { // This shouldn't happen.
		return "", nil
	}

	var execPath string
	switch runtime.GOOS {
	case "openbsd":
		// buf now contains **argv, with pointers to each of the C-style
		// NULL terminated arguments.
		var args []string
		argv := uintptr(unsafe.Pointer(&buf[0]))
	Loop:
		for {
			argp := *(**[1 << 20]byte)(unsafe.Pointer(argv))
			if argp == nil {
				break
			}
			for i := 0; uintptr(i) < n; i++ {
				// we don't want the full arguments list
				if string(argp[i]) == " " {
					break Loop
				}
				if argp[i] != 0 {
					continue
				}
				args = append(args, string(argp[:i]))
				n -= uintptr(i)
				break
			}
			if n < unsafe.Sizeof(argv) {
				break
			}
			argv += unsafe.Sizeof(argv)
			n -= unsafe.Sizeof(argv)
		}
		execPath = args[0]
		// There is no canonical way to get an executable path on
		// OpenBSD, so check PATH in case we are called directly
		if execPath[0] != '/' && execPath[0] != '.' {
			execIsInPath, err := exec.LookPath(execPath)
			if err == nil {
				execPath = execIsInPath
			}
		}
	default:
		for i, v := range buf {
			if v == 0 {
				buf = buf[:i]
				break
			}
		}
		execPath = string(buf)
	}

	var err error
	// execPath will not be empty due to above checks.
	// Try to get the absolute path if the execPath is not rooted.
	if execPath[0] != '/' {
		execPath, err = getAbs(execPath)
		if err != nil {
			return execPath, err
		}
	}
	// For darwin KERN_PROCARGS may return the path to a symlink rather than the
	// actual executable.
	if runtime.GOOS == "darwin" {
		if execPath, err = filepath.EvalSymlinks(execPath); err != nil {
			return execPath, err
		}
	}
	return execPath, nil
}

func getAbs(execPath string) (string, error) {
	if initCwdErr != nil {
		return execPath, initCwdErr
	}
	// The execPath may begin with a "../" or a "./" so clean it first.
	// Join the two paths, trailing and starting slashes undetermined, so use
	// the generic Join function.
	return filepath.Join(initCwd, filepath.Clean(execPath)), nil
}
