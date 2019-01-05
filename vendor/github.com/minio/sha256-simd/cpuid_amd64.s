// The MIT License (MIT)
//
// Copyright (c) 2015 Klaus Post
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// +build amd64,!gccgo

// func cpuid(op uint32) (eax, ebx, ecx, edx uint32)
TEXT ·cpuid(SB), 7, $0
	XORQ CX, CX
	MOVL op+0(FP), AX
	CPUID
	MOVL AX, eax+8(FP)
	MOVL BX, ebx+12(FP)
	MOVL CX, ecx+16(FP)
	MOVL DX, edx+20(FP)
	RET

// func cpuidex(op, op2 uint32) (eax, ebx, ecx, edx uint32)
TEXT ·cpuidex(SB), 7, $0
	MOVL op+0(FP), AX
	MOVL op2+4(FP), CX
	CPUID
	MOVL AX, eax+8(FP)
	MOVL BX, ebx+12(FP)
	MOVL CX, ecx+16(FP)
	MOVL DX, edx+20(FP)
	RET

// func xgetbv(index uint32) (eax, edx uint32)
TEXT ·xgetbv(SB), 7, $0
	MOVL index+0(FP), CX
	BYTE $0x0f; BYTE $0x01; BYTE $0xd0 // XGETBV
	MOVL AX, eax+8(FP)
	MOVL DX, edx+12(FP)
	RET
