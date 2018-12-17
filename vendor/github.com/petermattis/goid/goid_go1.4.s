// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Assembly to get into package runtime without using exported symbols.
// See https://github.com/golang/go/blob/release-branch.go1.4/misc/cgo/test/backdoor/thunk.s

// +build amd64 amd64p32 arm 386
// +build go1.4,!go1.5

#include "textflag.h"

#ifdef GOARCH_arm
#define JMP B
#endif

TEXT ·getg(SB),NOSPLIT,$0-0
	JMP	runtime·getg(SB)
