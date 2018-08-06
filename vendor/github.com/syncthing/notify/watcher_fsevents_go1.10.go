// Copyright (c) 2018 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build darwin,!kqueue,cgo,!go1.11

package notify

/*
 #include <CoreServices/CoreServices.h>
*/
import "C"

var refZero = (*C.struct___CFAllocator)(nil)
