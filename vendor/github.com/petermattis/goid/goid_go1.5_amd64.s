// Copyright 2016 Peter Mattis.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.

// Assembly to mimic runtime.getg.

// +build amd64 amd64p32
// +build gc,go1.5

#include "go_asm.h"
#include "textflag.h"

// func Get() int64
TEXT ·Get(SB),NOSPLIT,$0-8
	MOVQ (TLS), R14
	MOVQ g_goid(R14), R13
	MOVQ R13, ret+0(FP)
	RET
