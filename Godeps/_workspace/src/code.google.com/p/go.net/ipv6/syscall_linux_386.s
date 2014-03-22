// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This code is a duplicate of syscall/syscall_linux_386.s with small
// modifications.

#define SYS_SOCKETCALL	102	// from zsysnum_linux_386.go

// func socketcallnosplit7(call int, a0, a1, a2, a3, a4, a5 uintptr) (n int, errno int)
// Kernel interface gets call sub-number and pointer to a0 for Go 1.1.
TEXT ·socketcallnosplit7(SB),7,$0
	CALL	runtime·entersyscall(SB)
	MOVL	$SYS_SOCKETCALL, AX	// syscall entry
	MOVL	4(SP), BX		// socket call number
	LEAL	8(SP), CX		// pointer to call arguments
	MOVL	$0, DX
	MOVL	$0, SI
	MOVL	$0, DI
	CALL	*runtime·_vdso(SB)
	CMPL	AX, $0xfffff001
	JLS	ok1
	MOVL	$-1, 32(SP)		// n
	NEGL	AX
	MOVL	AX, 36(SP)		// errno
	CALL	runtime·exitsyscall(SB)
	RET
ok1:
	MOVL	AX, 32(SP)		// n
	MOVL	$0, 36(SP)		// errno
	CALL	runtime·exitsyscall(SB)
	RET

// func socketcallnosplit4(call int, a0, a1, a2, a3, a4, a5 uintptr) (n int, errno int)
// Kernel interface gets call sub-number and pointer to a0 for Go 1.2.
TEXT ·socketcallnosplit4(SB),4,$0-40
	CALL	runtime·entersyscall(SB)
	MOVL	$SYS_SOCKETCALL, AX	// syscall entry
	MOVL	4(SP), BX		// socket call number
	LEAL	8(SP), CX		// pointer to call arguments
	MOVL	$0, DX
	MOVL	$0, SI
	MOVL	$0, DI
	CALL	*runtime·_vdso(SB)
	CMPL	AX, $0xfffff001
	JLS	ok2
	MOVL	$-1, 32(SP)		// n
	NEGL	AX
	MOVL	AX, 36(SP)		// errno
	CALL	runtime·exitsyscall(SB)
	RET
ok2:
	MOVL	AX, 32(SP)		// n
	MOVL	$0, 36(SP)		// errno
	CALL	runtime·exitsyscall(SB)
	RET
