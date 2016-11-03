// Copyright (c) 2015 Klaus Post, released under MIT License. See LICENSE file.

// +build 386,!gccgo

// func cpuid(op uint32) (eax, ebx, ecx, edx uint32)
TEXT ·cpuid(SB), 7, $0
        XORL CX, CX
        MOVL op+0(FP), AX
        CPUID
        MOVL AX, eax+4(FP)
        MOVL BX, ebx+8(FP)
        MOVL CX, ecx+12(FP)
        MOVL DX, edx+16(FP)
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
        MOVL AX, eax+4(FP)
        MOVL DX, edx+8(FP)
        RET
