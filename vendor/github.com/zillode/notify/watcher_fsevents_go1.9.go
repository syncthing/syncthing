// Copyright (c) 2017 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build darwin,!kqueue,cgo,!go1.10

package notify

/*
#include <CoreServices/CoreServices.h>
*/
import "C"

var refZero = (*C.struct___CFAllocator)(nil)
