//+build !noasm !appengine

// SHA256 implementation for AVX2

//
// Minio Cloud Storage, (C) 2016 Minio, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

//
// This code is based on an Intel White-Paper:
// "Fast SHA-256 Implementations on Intel Architecture Processors"
//
// together with the reference implementation from the following authors:
//    James Guilford <james.guilford@intel.com>
//    Kirk Yap <kirk.s.yap@intel.com>
//    Tim Chen <tim.c.chen@linux.intel.com>
//
// For Golang it has been converted to Plan 9 assembly with the help of
// github.com/minio/asm2plan9s to assemble Intel instructions to their Plan9
// equivalents
//

#include "textflag.h"

DATA K256<>+0x000(SB)/8, $0x71374491428a2f98
DATA K256<>+0x008(SB)/8, $0xe9b5dba5b5c0fbcf
DATA K256<>+0x010(SB)/8, $0x71374491428a2f98
DATA K256<>+0x018(SB)/8, $0xe9b5dba5b5c0fbcf
DATA K256<>+0x020(SB)/8, $0x59f111f13956c25b
DATA K256<>+0x028(SB)/8, $0xab1c5ed5923f82a4
DATA K256<>+0x030(SB)/8, $0x59f111f13956c25b
DATA K256<>+0x038(SB)/8, $0xab1c5ed5923f82a4
DATA K256<>+0x040(SB)/8, $0x12835b01d807aa98
DATA K256<>+0x048(SB)/8, $0x550c7dc3243185be
DATA K256<>+0x050(SB)/8, $0x12835b01d807aa98
DATA K256<>+0x058(SB)/8, $0x550c7dc3243185be
DATA K256<>+0x060(SB)/8, $0x80deb1fe72be5d74
DATA K256<>+0x068(SB)/8, $0xc19bf1749bdc06a7
DATA K256<>+0x070(SB)/8, $0x80deb1fe72be5d74
DATA K256<>+0x078(SB)/8, $0xc19bf1749bdc06a7
DATA K256<>+0x080(SB)/8, $0xefbe4786e49b69c1
DATA K256<>+0x088(SB)/8, $0x240ca1cc0fc19dc6
DATA K256<>+0x090(SB)/8, $0xefbe4786e49b69c1
DATA K256<>+0x098(SB)/8, $0x240ca1cc0fc19dc6
DATA K256<>+0x0a0(SB)/8, $0x4a7484aa2de92c6f
DATA K256<>+0x0a8(SB)/8, $0x76f988da5cb0a9dc
DATA K256<>+0x0b0(SB)/8, $0x4a7484aa2de92c6f
DATA K256<>+0x0b8(SB)/8, $0x76f988da5cb0a9dc
DATA K256<>+0x0c0(SB)/8, $0xa831c66d983e5152
DATA K256<>+0x0c8(SB)/8, $0xbf597fc7b00327c8
DATA K256<>+0x0d0(SB)/8, $0xa831c66d983e5152
DATA K256<>+0x0d8(SB)/8, $0xbf597fc7b00327c8
DATA K256<>+0x0e0(SB)/8, $0xd5a79147c6e00bf3
DATA K256<>+0x0e8(SB)/8, $0x1429296706ca6351
DATA K256<>+0x0f0(SB)/8, $0xd5a79147c6e00bf3
DATA K256<>+0x0f8(SB)/8, $0x1429296706ca6351
DATA K256<>+0x100(SB)/8, $0x2e1b213827b70a85
DATA K256<>+0x108(SB)/8, $0x53380d134d2c6dfc
DATA K256<>+0x110(SB)/8, $0x2e1b213827b70a85
DATA K256<>+0x118(SB)/8, $0x53380d134d2c6dfc
DATA K256<>+0x120(SB)/8, $0x766a0abb650a7354
DATA K256<>+0x128(SB)/8, $0x92722c8581c2c92e
DATA K256<>+0x130(SB)/8, $0x766a0abb650a7354
DATA K256<>+0x138(SB)/8, $0x92722c8581c2c92e
DATA K256<>+0x140(SB)/8, $0xa81a664ba2bfe8a1
DATA K256<>+0x148(SB)/8, $0xc76c51a3c24b8b70
DATA K256<>+0x150(SB)/8, $0xa81a664ba2bfe8a1
DATA K256<>+0x158(SB)/8, $0xc76c51a3c24b8b70
DATA K256<>+0x160(SB)/8, $0xd6990624d192e819
DATA K256<>+0x168(SB)/8, $0x106aa070f40e3585
DATA K256<>+0x170(SB)/8, $0xd6990624d192e819
DATA K256<>+0x178(SB)/8, $0x106aa070f40e3585
DATA K256<>+0x180(SB)/8, $0x1e376c0819a4c116
DATA K256<>+0x188(SB)/8, $0x34b0bcb52748774c
DATA K256<>+0x190(SB)/8, $0x1e376c0819a4c116
DATA K256<>+0x198(SB)/8, $0x34b0bcb52748774c
DATA K256<>+0x1a0(SB)/8, $0x4ed8aa4a391c0cb3
DATA K256<>+0x1a8(SB)/8, $0x682e6ff35b9cca4f
DATA K256<>+0x1b0(SB)/8, $0x4ed8aa4a391c0cb3
DATA K256<>+0x1b8(SB)/8, $0x682e6ff35b9cca4f
DATA K256<>+0x1c0(SB)/8, $0x78a5636f748f82ee
DATA K256<>+0x1c8(SB)/8, $0x8cc7020884c87814
DATA K256<>+0x1d0(SB)/8, $0x78a5636f748f82ee
DATA K256<>+0x1d8(SB)/8, $0x8cc7020884c87814
DATA K256<>+0x1e0(SB)/8, $0xa4506ceb90befffa
DATA K256<>+0x1e8(SB)/8, $0xc67178f2bef9a3f7
DATA K256<>+0x1f0(SB)/8, $0xa4506ceb90befffa
DATA K256<>+0x1f8(SB)/8, $0xc67178f2bef9a3f7

DATA K256<>+0x200(SB)/8, $0x0405060700010203
DATA K256<>+0x208(SB)/8, $0x0c0d0e0f08090a0b
DATA K256<>+0x210(SB)/8, $0x0405060700010203
DATA K256<>+0x218(SB)/8, $0x0c0d0e0f08090a0b
DATA K256<>+0x220(SB)/8, $0x0b0a090803020100
DATA K256<>+0x228(SB)/8, $0xffffffffffffffff
DATA K256<>+0x230(SB)/8, $0x0b0a090803020100
DATA K256<>+0x238(SB)/8, $0xffffffffffffffff
DATA K256<>+0x240(SB)/8, $0xffffffffffffffff
DATA K256<>+0x248(SB)/8, $0x0b0a090803020100
DATA K256<>+0x250(SB)/8, $0xffffffffffffffff
DATA K256<>+0x258(SB)/8, $0x0b0a090803020100

GLOBL K256<>(SB), 8, $608

// func blockAvx2(h []uint32, message []uint8)
TEXT ·blockAvx2(SB), 7, $0

    MOVQ  ctx+0(FP), DI                        // DI: &h
    MOVQ  inp+24(FP), SI                       // SI: &message
    MOVQ  inplength+32(FP), DX                 // len(message)
    ADDQ  SI, DX                               // end pointer of input
    MOVQ  SP, R11                              // copy stack pointer
    SUBQ  $0x220, SP                           // sp -= 0x220
    ANDQ  $0xfffffffffffffc00, SP              // align stack frame
    ADDQ  $0x1c0, SP
    MOVQ DI, 0x40(SP)                          // save ctx
    MOVQ SI, 0x48(SP)                          // save input
    MOVQ DX, 0x50(SP)                          // save end pointer
    MOVQ R11, 0x58(SP)                         // save copy of stack pointer

    WORD $0xf8c5; BYTE $0x77                   // vzeroupper
    ADDQ $0x40, SI			                   // input++
    MOVL (DI), AX
    MOVQ SI, R12                               // borrow $T1
    MOVL 4(DI), BX
    CMPQ SI, DX                                // $_end
    MOVL 8(DI), CX
    LONG $0xe4440f4c                           // cmove  r12,rsp            /* next block or random data */
    MOVL 12(DI), DX
    MOVL 16(DI), R8
    MOVL 20(DI), R9
    MOVL 24(DI), R10
    MOVL 28(DI), R11

    LEAQ K256<>(SB), BP
    LONG $0x856f7dc5; LONG $0x00000220         // VMOVDQA YMM8, 0x220[rbp]  /* vmovdqa ymm8,YMMWORD PTR [rip+0x220] */
    LONG $0x8d6f7dc5; LONG $0x00000240         // VMOVDQA YMM9, 0x240[rbp]  /* vmovdqa ymm9,YMMWORD PTR [rip+0x240] */
    LONG $0x956f7dc5; LONG $0x00000200         // VMOVDQA YMM10, 0x200[rbp] /* vmovdqa ymm7,YMMWORD PTR [rip+0x200] */

loop0:
    LONG $0x6f7dc1c4; BYTE $0xfa               // VMOVDQA YMM7, YMM10

    // Load first 16 dwords from two blocks
    MOVOU -64(SI), X0                          // vmovdqu xmm0,XMMWORD PTR [rsi-0x40]
    MOVOU -48(SI), X1                          // vmovdqu xmm1,XMMWORD PTR [rsi-0x30]
    MOVOU -32(SI), X2                          // vmovdqu xmm2,XMMWORD PTR [rsi-0x20]
    MOVOU -16(SI), X3                          // vmovdqu xmm3,XMMWORD PTR [rsi-0x10]

    // Byte swap data and transpose data into high/low
    LONG $0x387dc3c4; WORD $0x2404; BYTE $0x01 // vinserti128 ymm0,ymm0,[r12],0x1
    LONG $0x3875c3c4; LONG $0x0110244c         // vinserti128 ymm1,ymm1,0x10[r12],0x1
    LONG $0x007de2c4; BYTE $0xc7               // vpshufb     ymm0,ymm0,ymm7
    LONG $0x386dc3c4; LONG $0x01202454         // vinserti128 ymm2,ymm2,0x20[r12],0x1
    LONG $0x0075e2c4; BYTE $0xcf               // vpshufb     ymm1,ymm1,ymm7
    LONG $0x3865c3c4; LONG $0x0130245c         // vinserti128 ymm3,ymm3,0x30[r12],0x1

    LEAQ K256<>(SB), BP
    LONG $0x006de2c4; BYTE $0xd7               // vpshufb ymm2,ymm2,ymm7
    LONG $0x65fefdc5; BYTE $0x00               // vpaddd  ymm4,ymm0,[rbp]
    LONG $0x0065e2c4; BYTE $0xdf               // vpshufb ymm3,ymm3,ymm7
    LONG $0x6dfef5c5; BYTE $0x20               // vpaddd  ymm5,ymm1,0x20[rbp]
    LONG $0x75feedc5; BYTE $0x40               // vpaddd  ymm6,ymm2,0x40[rbp]
    LONG $0x7dfee5c5; BYTE $0x60               // vpaddd  ymm7,ymm3,0x60[rbp]

    LONG $0x247ffdc5; BYTE $0x24               // vmovdqa [rsp],ymm4
    XORQ R14, R14
    LONG $0x6c7ffdc5; WORD $0x2024             // vmovdqa [rsp+0x20],ymm5

    ADDQ $-0x40, SP
    MOVQ BX, DI
    LONG $0x347ffdc5; BYTE $0x24               // vmovdqa [rsp],ymm6
    XORQ CX, DI                                // magic
    LONG $0x7c7ffdc5; WORD $0x2024             // vmovdqa [rsp+0x20],ymm7
    MOVQ R9, R12
    ADDQ $0x80,BP

loop1:
    // Schedule 48 input dwords, by doing 3 rounds of 12 each
    // Note: SIMD instructions are interleaved with the SHA calculations
    ADDQ $-0x40, SP
    LONG $0x0f75e3c4; WORD $0x04e0 // vpalignr ymm4,ymm1,ymm0,0x4

    // ROUND(AX, BX, CX, DX, R8, R9, R10, R11, R12, R13, R14, R15, DI, SP, 0x80)
    LONG $0x249c0344; LONG $0x00000080 // add    r11d,[rsp+0x80]
    WORD $0x2145; BYTE $0xc4       // and    r12d,r8d
    LONG $0xf07b43c4; WORD $0x19e8 // rorx   r13d,r8d,0x19
    LONG $0x0f65e3c4; WORD $0x04fa // vpalignr ymm7,ymm3,ymm2,0x4
    LONG $0xf07b43c4; WORD $0x0bf8 // rorx   r15d,r8d,0xb
    LONG $0x30048d42               // lea    eax,[rax+r14*1]            
    LONG $0x231c8d47               // lea    r11d,[r11+r12*1]
    LONG $0xd472cdc5; BYTE $0x07   // vpsrld ymm6,ymm4,0x7
    LONG $0xf23842c4; BYTE $0xe2   // andn   r12d,r8d,r10d
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b43c4; WORD $0x06f0 // rorx   r14d,r8d,0x6
    LONG $0xc7fefdc5               // vpaddd ymm0,ymm0,ymm7
    LONG $0x231c8d47               // lea    r11d,[r11+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8941; BYTE $0xc7       // mov    r15d,eax
    LONG $0xd472c5c5; BYTE $0x03   // vpsrld ymm7,ymm4,0x3
    LONG $0xf07b63c4; WORD $0x16e0 // rorx   r12d,eax,0x16
    LONG $0x2b1c8d47               // lea    r11d,[r11+r13*1]
    WORD $0x3141; BYTE $0xdf       // xor    r15d,ebx
    LONG $0xf472d5c5; BYTE $0x0e   // vpslld ymm5,ymm4,0xe
    LONG $0xf07b63c4; WORD $0x0df0 // rorx   r14d,eax,0xd
    LONG $0xf07b63c4; WORD $0x02e8 // rorx   r13d,eax,0x2
    LONG $0x1a148d42               // lea    edx,[rdx+r11*1]
    LONG $0xe6efc5c5               // vpxor  ymm4,ymm7,ymm6
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0xdf31                   // xor    edi,ebx
    LONG $0xfb70fdc5; BYTE $0xfa   // vpshufd ymm7,ymm3,0xfa
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x3b1c8d45               // lea    r11d,[r11+rdi*1]
    WORD $0x8945; BYTE $0xc4       // mov    r12d,r8d
    LONG $0xd672cdc5; BYTE $0x0b   // vpsrld ymm6,ymm6,0xb

    // ROUND(R11, AX, BX, CX, DX, R8, R9, R10, R12, R13, R14, DI, R15, SP, 0x84)
    LONG $0x24940344; LONG $0x00000084 // add    r10d,[rsp+0x84]
    WORD $0x2141; BYTE $0xd4       // and    r12d,edx
    LONG $0xf07b63c4; WORD $0x19ea // rorx   r13d,edx,0x19
    LONG $0xe5efddc5               // vpxor  ymm4,ymm4,ymm5
    LONG $0xf07be3c4; WORD $0x0bfa // rorx   edi,edx,0xb
    LONG $0x331c8d47               // lea    r11d,[r11+r14*1]
    LONG $0x22148d47               // lea    r10d,[r10+r12*1]
    LONG $0xf572d5c5; BYTE $0x0b   // vpslld ymm5,ymm5,0xb
    LONG $0xf26842c4; BYTE $0xe1   // andn   r12d,edx,r9d
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b63c4; WORD $0x06f2 // rorx   r14d,edx,0x6
    LONG $0xe6efddc5               // vpxor  ymm4,ymm4,ymm6
    LONG $0x22148d47               // lea    r10d,[r10+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8944; BYTE $0xdf       // mov    edi,r11d
    LONG $0xd772cdc5; BYTE $0x0a   // vpsrld ymm6,ymm7,0xa
    LONG $0xf07b43c4; WORD $0x16e3 // rorx   r12d,r11d,0x16
    LONG $0x2a148d47               // lea    r10d,[r10+r13*1]
    WORD $0xc731                   // xor    edi,eax
    LONG $0xe5efddc5               // vpxor  ymm4,ymm4,ymm5
    LONG $0xf07b43c4; WORD $0x0df3 // rorx   r14d,r11d,0xd
    LONG $0xf07b43c4; WORD $0x02eb // rorx   r13d,r11d,0x2
    LONG $0x110c8d42               // lea    ecx,[rcx+r10*1]
    LONG $0xd773c5c5; BYTE $0x11   // vpsrlq ymm7,ymm7,0x11
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3141; BYTE $0xc7       // xor    r15d,eax
    LONG $0xc4fefdc5               // vpaddd ymm0,ymm0,ymm4
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x3a148d47               // lea    r10d,[r10+r15*1]
    WORD $0x8941; BYTE $0xd4       // mov    r12d,edx
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7

    // ROUND(R10, R11, AX, BX, CX, DX, R8, R9, R12, R13, R14, R15, DI, SP, 0x88)
    LONG $0x248c0344; LONG $0x00000088 // add    r9d,[rsp+0x88]
    WORD $0x2141; BYTE $0xcc       // and    r12d,ecx
    LONG $0xf07b63c4; WORD $0x19e9 // rorx   r13d,ecx,0x19
    LONG $0xd773c5c5; BYTE $0x02   // vpsrlq ymm7,ymm7,0x2
    LONG $0xf07b63c4; WORD $0x0bf9 // rorx   r15d,ecx,0xb
    LONG $0x32148d47               // lea    r10d,[r10+r14*1]
    LONG $0x210c8d47               // lea    r9d,[r9+r12*1]
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7
    LONG $0xf27042c4; BYTE $0xe0   // andn   r12d,ecx,r8d
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b63c4; WORD $0x06f1 // rorx   r14d,ecx,0x6
    LONG $0x004dc2c4; BYTE $0xf0   // vpshufb ymm6,ymm6,ymm8
    LONG $0x210c8d47               // lea    r9d,[r9+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8945; BYTE $0xd7       // mov    r15d,r10d
    LONG $0xc6fefdc5               // vpaddd ymm0,ymm0,ymm6
    LONG $0xf07b43c4; WORD $0x16e2 // rorx   r12d,r10d,0x16
    LONG $0x290c8d47               // lea    r9d,[r9+r13*1]
    WORD $0x3145; BYTE $0xdf       // xor    r15d,r11d
    LONG $0xf870fdc5; BYTE $0x50   // vpshufd ymm7,ymm0,0x50
    LONG $0xf07b43c4; WORD $0x0df2 // rorx   r14d,r10d,0xd
    LONG $0xf07b43c4; WORD $0x02ea // rorx   r13d,r10d,0x2
    LONG $0x0b1c8d42               // lea    ebx,[rbx+r9*1]
    LONG $0xd772cdc5; BYTE $0x0a   // vpsrld ymm6,ymm7,0xa
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3144; BYTE $0xdf       // xor    edi,r11d
    LONG $0xd773c5c5; BYTE $0x11   // vpsrlq ymm7,ymm7,0x11
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x390c8d45               // lea    r9d,[r9+rdi*1]
    WORD $0x8941; BYTE $0xcc       // mov    r12d,ecx
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7

    // ROUND(R9, R10, R11, AX, BX, CX, DX, R8, R12, R13, R14, DI, R15, SP, 0x8c)
    LONG $0x24840344; LONG $0x0000008c // add    r8d,[rsp+0x8c]
    WORD $0x2141; BYTE $0xdc       // and    r12d,ebx
    LONG $0xf07b63c4; WORD $0x19eb // rorx   r13d,ebx,0x19
    LONG $0xd773c5c5; BYTE $0x02   // vpsrlq ymm7,ymm7,0x2
    LONG $0xf07be3c4; WORD $0x0bfb // rorx   edi,ebx,0xb
    LONG $0x310c8d47               // lea    r9d,[r9+r14*1]
    LONG $0x20048d47               // lea    r8d,[r8+r12*1]
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7
    LONG $0xf26062c4; BYTE $0xe2   // andn   r12d,ebx,edx
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b63c4; WORD $0x06f3 // rorx   r14d,ebx,0x6
    LONG $0x004dc2c4; BYTE $0xf1   // vpshufb ymm6,ymm6,ymm9
    LONG $0x20048d47               // lea    r8d,[r8+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8944; BYTE $0xcf       // mov    edi,r9d
    LONG $0xc6fefdc5               // vpaddd ymm0,ymm0,ymm6
    LONG $0xf07b43c4; WORD $0x16e1 // rorx   r12d,r9d,0x16
    LONG $0x28048d47               // lea    r8d,[r8+r13*1]
    WORD $0x3144; BYTE $0xd7       // xor    edi,r10d
    LONG $0x75fefdc5; BYTE $0x00   // vpaddd ymm6,ymm0,[rbp+0x0]
    LONG $0xf07b43c4; WORD $0x0df1 // rorx   r14d,r9d,0xd
    LONG $0xf07b43c4; WORD $0x02e9 // rorx   r13d,r9d,0x2
    LONG $0x00048d42               // lea    eax,[rax+r8*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3145; BYTE $0xd7       // xor    r15d,r10d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x38048d47               // lea    r8d,[r8+r15*1]
    WORD $0x8941; BYTE $0xdc       // mov    r12d,ebx

    LONG $0x347ffdc5; BYTE $0x24 // vmovdqa [rsp],ymm6
    LONG $0x0f6de3c4; WORD $0x04e1 // vpalignr ymm4,ymm2,ymm1,0x4

    // ROUND(R8, R9, R10, R11, AX, BX, CX, DX, R12, R13, R14, R15, DI, SP, 0xa0)
    LONG $0xa0249403; WORD $0x0000; BYTE $0x00 // add    edx,[rsp+0xa0]
    WORD $0x2141; BYTE $0xc4       // and    r12d,eax
    LONG $0xf07b63c4; WORD $0x19e8 // rorx   r13d,eax,0x19
    LONG $0x0f7de3c4; WORD $0x04fb // vpalignr ymm7,ymm0,ymm3,0x4
    LONG $0xf07b63c4; WORD $0x0bf8 // rorx   r15d,eax,0xb
    LONG $0x30048d47               // lea    r8d,[r8+r14*1]
    LONG $0x22148d42               // lea    edx,[rdx+r12*1]
    LONG $0xd472cdc5; BYTE $0x07   // vpsrld ymm6,ymm4,0x7
    LONG $0xf27862c4; BYTE $0xe1   // andn   r12d,eax,ecx
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b63c4; WORD $0x06f0 // rorx   r14d,eax,0x6
    LONG $0xcffef5c5               // vpaddd ymm1,ymm1,ymm7
    LONG $0x22148d42               // lea    edx,[rdx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8945; BYTE $0xc7       // mov    r15d,r8d
    LONG $0xd472c5c5; BYTE $0x03   // vpsrld ymm7,ymm4,0x3
    LONG $0xf07b43c4; WORD $0x16e0 // rorx   r12d,r8d,0x16
    LONG $0x2a148d42               // lea    edx,[rdx+r13*1]
    WORD $0x3145; BYTE $0xcf       // xor    r15d,r9d
    LONG $0xf472d5c5; BYTE $0x0e   // vpslld ymm5,ymm4,0xe
    LONG $0xf07b43c4; WORD $0x0df0 // rorx   r14d,r8d,0xd
    LONG $0xf07b43c4; WORD $0x02e8 // rorx   r13d,r8d,0x2
    LONG $0x131c8d45               // lea    r11d,[r11+rdx*1]
    LONG $0xe6efc5c5               // vpxor  ymm4,ymm7,ymm6
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3144; BYTE $0xcf       // xor    edi,r9d
    LONG $0xf870fdc5; BYTE $0xfa   // vpshufd ymm7,ymm0,0xfa
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    WORD $0x148d; BYTE $0x3a       // lea    edx,[rdx+rdi*1]
    WORD $0x8941; BYTE $0xc4       // mov    r12d,eax
    LONG $0xd672cdc5; BYTE $0x0b   // vpsrld ymm6,ymm6,0xb

    // ROUND(DX, R8, R9, R10, R11, AX, BX, CX, R12, R13, R14, DI, R15, SP, 0xa4)
    LONG $0xa4248c03; WORD $0x0000; BYTE $0x00 // add    ecx,[rsp+0xa4]
    WORD $0x2145; BYTE $0xdc       // and    r12d,r11d
    LONG $0xf07b43c4; WORD $0x19eb // rorx   r13d,r11d,0x19
    LONG $0xe5efddc5               // vpxor  ymm4,ymm4,ymm5
    LONG $0xf07bc3c4; WORD $0x0bfb // rorx   edi,r11d,0xb
    LONG $0x32148d42               // lea    edx,[rdx+r14*1]
    LONG $0x210c8d42               // lea    ecx,[rcx+r12*1]
    LONG $0xf572d5c5; BYTE $0x0b   // vpslld ymm5,ymm5,0xb
    LONG $0xf22062c4; BYTE $0xe3   // andn   r12d,r11d,ebx
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b43c4; WORD $0x06f3 // rorx   r14d,r11d,0x6
    LONG $0xe6efddc5               // vpxor  ymm4,ymm4,ymm6
    LONG $0x210c8d42               // lea    ecx,[rcx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0xd789                   // mov    edi,edx
    LONG $0xd772cdc5; BYTE $0x0a   // vpsrld ymm6,ymm7,0xa
    LONG $0xf07b63c4; WORD $0x16e2 // rorx   r12d,edx,0x16
    LONG $0x290c8d42               // lea    ecx,[rcx+r13*1]
    WORD $0x3144; BYTE $0xc7       // xor    edi,r8d
    LONG $0xe5efddc5               // vpxor  ymm4,ymm4,ymm5
    LONG $0xf07b63c4; WORD $0x0df2 // rorx   r14d,edx,0xd
    LONG $0xf07b63c4; WORD $0x02ea // rorx   r13d,edx,0x2
    LONG $0x0a148d45               // lea    r10d,[r10+rcx*1]
    LONG $0xd773c5c5; BYTE $0x11   // vpsrlq ymm7,ymm7,0x11
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3145; BYTE $0xc7       // xor    r15d,r8d
    LONG $0xccfef5c5               // vpaddd ymm1,ymm1,ymm4
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x390c8d42               // lea    ecx,[rcx+r15*1]
    WORD $0x8945; BYTE $0xdc       // mov    r12d,r11d
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7

    // ROUND(CX, DX, R8, R9, R10, R11, AX, BX, R12, R13, R14, R15, DI, SP, 0xa8)
    LONG $0xa8249c03; WORD $0x0000; BYTE $0x00 // add    ebx,[rsp+0xa8]
    WORD $0x2145; BYTE $0xd4       // and    r12d,r10d
    LONG $0xf07b43c4; WORD $0x19ea // rorx   r13d,r10d,0x19
    LONG $0xd773c5c5; BYTE $0x02   // vpsrlq ymm7,ymm7,0x2
    LONG $0xf07b43c4; WORD $0x0bfa // rorx   r15d,r10d,0xb
    LONG $0x310c8d42               // lea    ecx,[rcx+r14*1]
    LONG $0x231c8d42               // lea    ebx,[rbx+r12*1]
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7
    LONG $0xf22862c4; BYTE $0xe0   // andn   r12d,r10d,eax
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b43c4; WORD $0x06f2 // rorx   r14d,r10d,0x6
    LONG $0x004dc2c4; BYTE $0xf0   // vpshufb ymm6,ymm6,ymm8
    LONG $0x231c8d42               // lea    ebx,[rbx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8941; BYTE $0xcf       // mov    r15d,ecx
    LONG $0xcefef5c5               // vpaddd ymm1,ymm1,ymm6
    LONG $0xf07b63c4; WORD $0x16e1 // rorx   r12d,ecx,0x16
    LONG $0x2b1c8d42               // lea    ebx,[rbx+r13*1]
    WORD $0x3141; BYTE $0xd7       // xor    r15d,edx
    LONG $0xf970fdc5; BYTE $0x50   // vpshufd ymm7,ymm1,0x50
    LONG $0xf07b63c4; WORD $0x0df1 // rorx   r14d,ecx,0xd
    LONG $0xf07b63c4; WORD $0x02e9 // rorx   r13d,ecx,0x2
    LONG $0x190c8d45               // lea    r9d,[r9+rbx*1]
    LONG $0xd772cdc5; BYTE $0x0a   // vpsrld ymm6,ymm7,0xa
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0xd731                   // xor    edi,edx
    LONG $0xd773c5c5; BYTE $0x11   // vpsrlq ymm7,ymm7,0x11
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    WORD $0x1c8d; BYTE $0x3b       // lea    ebx,[rbx+rdi*1]
    WORD $0x8945; BYTE $0xd4       // mov    r12d,r10d
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7

    // ROUND(BX, CX, DX, R8, R9, R10, R11, AX, R12, R13, R14, DI, R15, SP, 0xac)
    LONG $0xac248403; WORD $0x0000; BYTE $0x00 // add    eax,[rsp+0xac]
    WORD $0x2145; BYTE $0xcc       // and    r12d,r9d
    LONG $0xf07b43c4; WORD $0x19e9 // rorx   r13d,r9d,0x19
    LONG $0xd773c5c5; BYTE $0x02   // vpsrlq ymm7,ymm7,0x2
    LONG $0xf07bc3c4; WORD $0x0bf9 // rorx   edi,r9d,0xb
    LONG $0x331c8d42               // lea    ebx,[rbx+r14*1]
    LONG $0x20048d42               // lea    eax,[rax+r12*1]
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7
    LONG $0xf23042c4; BYTE $0xe3   // andn   r12d,r9d,r11d
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b43c4; WORD $0x06f1 // rorx   r14d,r9d,0x6
    LONG $0x004dc2c4; BYTE $0xf1   // vpshufb ymm6,ymm6,ymm9
    LONG $0x20048d42               // lea    eax,[rax+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0xdf89                   // mov    edi,ebx
    LONG $0xcefef5c5               // vpaddd ymm1,ymm1,ymm6
    LONG $0xf07b63c4; WORD $0x16e3 // rorx   r12d,ebx,0x16
    LONG $0x28048d42               // lea    eax,[rax+r13*1]
    WORD $0xcf31                   // xor    edi,ecx
    LONG $0x75fef5c5; BYTE $0x20   // vpaddd ymm6,ymm1,[rbp+0x20]
    LONG $0xf07b63c4; WORD $0x0df3 // rorx   r14d,ebx,0xd
    LONG $0xf07b63c4; WORD $0x02eb // rorx   r13d,ebx,0x2
    LONG $0x00048d45               // lea    r8d,[r8+rax*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3141; BYTE $0xcf       // xor    r15d,ecx
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x38048d42               // lea    eax,[rax+r15*1]
    WORD $0x8945; BYTE $0xcc       // mov    r12d,r9d

    LONG $0x747ffdc5; WORD $0x2024 // vmovdqa [rsp+0x20],ymm6

    LONG $0x24648d48; BYTE $0xc0   // lea    rsp,[rsp-0x40]
    LONG $0x0f65e3c4; WORD $0x04e2 // vpalignr ymm4,ymm3,ymm2,0x4

    // ROUND(AX, BX, CX, DX, R8, R9, R10, R11, R12, R13, R14, R15, DI, SP, 0x80)
    LONG $0x249c0344; LONG $0x00000080 // add    r11d,[rsp+0x80]
    WORD $0x2145; BYTE $0xc4       // and    r12d,r8d
    LONG $0xf07b43c4; WORD $0x19e8 // rorx   r13d,r8d,0x19
    LONG $0x0f75e3c4; WORD $0x04f8 // vpalignr ymm7,ymm1,ymm0,0x4
    LONG $0xf07b43c4; WORD $0x0bf8 // rorx   r15d,r8d,0xb
    LONG $0x30048d42               // lea    eax,[rax+r14*1]
    LONG $0x231c8d47               // lea    r11d,[r11+r12*1]
    LONG $0xd472cdc5; BYTE $0x07   // vpsrld ymm6,ymm4,0x7
    LONG $0xf23842c4; BYTE $0xe2   // andn   r12d,r8d,r10d
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b43c4; WORD $0x06f0 // rorx   r14d,r8d,0x6
    LONG $0xd7feedc5               // vpaddd ymm2,ymm2,ymm7
    LONG $0x231c8d47               // lea    r11d,[r11+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8941; BYTE $0xc7       // mov    r15d,eax
    LONG $0xd472c5c5; BYTE $0x03   // vpsrld ymm7,ymm4,0x3
    LONG $0xf07b63c4; WORD $0x16e0 // rorx   r12d,eax,0x16
    LONG $0x2b1c8d47               // lea    r11d,[r11+r13*1]
    WORD $0x3141; BYTE $0xdf       // xor    r15d,ebx
    LONG $0xf472d5c5; BYTE $0x0e   // vpslld ymm5,ymm4,0xe
    LONG $0xf07b63c4; WORD $0x0df0 // rorx   r14d,eax,0xd
    LONG $0xf07b63c4; WORD $0x02e8 // rorx   r13d,eax,0x2
    LONG $0x1a148d42               // lea    edx,[rdx+r11*1]
    LONG $0xe6efc5c5               // vpxor  ymm4,ymm7,ymm6
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0xdf31                   // xor    edi,ebx
    LONG $0xf970fdc5; BYTE $0xfa   // vpshufd ymm7,ymm1,0xfa
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x3b1c8d45               // lea    r11d,[r11+rdi*1]
    WORD $0x8945; BYTE $0xc4       // mov    r12d,r8d
    LONG $0xd672cdc5; BYTE $0x0b   // vpsrld ymm6,ymm6,0xb

    // ROUND(R11, AX, BX, CX, DX, R8, R9, R10, R12, R13, R14, DI, R15, SP, 0x84)
    LONG $0x24940344; LONG $0x00000084 // add    r10d,[rsp+0x84]
    WORD $0x2141; BYTE $0xd4       // and    r12d,edx
    LONG $0xf07b63c4; WORD $0x19ea // rorx   r13d,edx,0x19
    LONG $0xe5efddc5               // vpxor  ymm4,ymm4,ymm5
    LONG $0xf07be3c4; WORD $0x0bfa // rorx   edi,edx,0xb
    LONG $0x331c8d47               // lea    r11d,[r11+r14*1]
    LONG $0x22148d47               // lea    r10d,[r10+r12*1]
    LONG $0xf572d5c5; BYTE $0x0b   // vpslld ymm5,ymm5,0xb
    LONG $0xf26842c4; BYTE $0xe1   // andn   r12d,edx,r9d
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b63c4; WORD $0x06f2 // rorx   r14d,edx,0x6
    LONG $0xe6efddc5               // vpxor  ymm4,ymm4,ymm6
    LONG $0x22148d47               // lea    r10d,[r10+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8944; BYTE $0xdf       // mov    edi,r11d
    LONG $0xd772cdc5; BYTE $0x0a   // vpsrld ymm6,ymm7,0xa
    LONG $0xf07b43c4; WORD $0x16e3 // rorx   r12d,r11d,0x16
    LONG $0x2a148d47               // lea    r10d,[r10+r13*1]
    WORD $0xc731                   // xor    edi,eax
    LONG $0xe5efddc5               // vpxor  ymm4,ymm4,ymm5
    LONG $0xf07b43c4; WORD $0x0df3 // rorx   r14d,r11d,0xd
    LONG $0xf07b43c4; WORD $0x02eb // rorx   r13d,r11d,0x2
    LONG $0x110c8d42               // lea    ecx,[rcx+r10*1]
    LONG $0xd773c5c5; BYTE $0x11   // vpsrlq ymm7,ymm7,0x11
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3141; BYTE $0xc7       // xor    r15d,eax
    LONG $0xd4feedc5               // vpaddd ymm2,ymm2,ymm4
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x3a148d47               // lea    r10d,[r10+r15*1]
    WORD $0x8941; BYTE $0xd4       // mov    r12d,edx
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7

    // ROUND(R10, R11, AX, BX, CX, DX, R8, R9, R12, R13, R14, R15, DI, SP, 0x88)
    LONG $0x248c0344; LONG $0x00000088 // add    r9d,[rsp+0x88]
    WORD $0x2141; BYTE $0xcc       // and    r12d,ecx
    LONG $0xf07b63c4; WORD $0x19e9 // rorx   r13d,ecx,0x19
    LONG $0xd773c5c5; BYTE $0x02   // vpsrlq ymm7,ymm7,0x2
    LONG $0xf07b63c4; WORD $0x0bf9 // rorx   r15d,ecx,0xb
    LONG $0x32148d47               // lea    r10d,[r10+r14*1]
    LONG $0x210c8d47               // lea    r9d,[r9+r12*1]
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7
    LONG $0xf27042c4; BYTE $0xe0   // andn   r12d,ecx,r8d
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b63c4; WORD $0x06f1 // rorx   r14d,ecx,0x6
    LONG $0x004dc2c4; BYTE $0xf0   // vpshufb ymm6,ymm6,ymm8
    LONG $0x210c8d47               // lea    r9d,[r9+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8945; BYTE $0xd7       // mov    r15d,r10d
    LONG $0xd6feedc5               // vpaddd ymm2,ymm2,ymm6
    LONG $0xf07b43c4; WORD $0x16e2 // rorx   r12d,r10d,0x16
    LONG $0x290c8d47               // lea    r9d,[r9+r13*1]
    WORD $0x3145; BYTE $0xdf       // xor    r15d,r11d
    LONG $0xfa70fdc5; BYTE $0x50   // vpshufd ymm7,ymm2,0x50
    LONG $0xf07b43c4; WORD $0x0df2 // rorx   r14d,r10d,0xd
    LONG $0xf07b43c4; WORD $0x02ea // rorx   r13d,r10d,0x2
    LONG $0x0b1c8d42               // lea    ebx,[rbx+r9*1]
    LONG $0xd772cdc5; BYTE $0x0a   // vpsrld ymm6,ymm7,0xa
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3144; BYTE $0xdf       // xor    edi,r11d
    LONG $0xd773c5c5; BYTE $0x11   // vpsrlq ymm7,ymm7,0x11
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x390c8d45               // lea    r9d,[r9+rdi*1]
    WORD $0x8941; BYTE $0xcc       // mov    r12d,ecx
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7

    // ROUND(R9, R10, R11, AX, BX, CX, DX, R8, R12, R13, R14, DI, R15, SP, 0x8c)
    LONG $0x24840344; LONG $0x0000008c // add    r8d,[rsp+0x8c]
    WORD $0x2141; BYTE $0xdc       // and    r12d,ebx
    LONG $0xf07b63c4; WORD $0x19eb // rorx   r13d,ebx,0x19
    LONG $0xd773c5c5; BYTE $0x02   // vpsrlq ymm7,ymm7,0x2
    LONG $0xf07be3c4; WORD $0x0bfb // rorx   edi,ebx,0xb
    LONG $0x310c8d47               // lea    r9d,[r9+r14*1]
    LONG $0x20048d47               // lea    r8d,[r8+r12*1]
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7
    LONG $0xf26062c4; BYTE $0xe2   // andn   r12d,ebx,edx
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b63c4; WORD $0x06f3 // rorx   r14d,ebx,0x6
    LONG $0x004dc2c4; BYTE $0xf1   // vpshufb ymm6,ymm6,ymm9
    LONG $0x20048d47               // lea    r8d,[r8+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8944; BYTE $0xcf       // mov    edi,r9d
    LONG $0xd6feedc5               // vpaddd ymm2,ymm2,ymm6
    LONG $0xf07b43c4; WORD $0x16e1 // rorx   r12d,r9d,0x16
    LONG $0x28048d47               // lea    r8d,[r8+r13*1]
    WORD $0x3144; BYTE $0xd7       // xor    edi,r10d
    LONG $0x75feedc5; BYTE $0x40   // vpaddd ymm6,ymm2,[rbp+0x40]
    LONG $0xf07b43c4; WORD $0x0df1 // rorx   r14d,r9d,0xd
    LONG $0xf07b43c4; WORD $0x02e9 // rorx   r13d,r9d,0x2
    LONG $0x00048d42               // lea    eax,[rax+r8*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3145; BYTE $0xd7       // xor    r15d,r10d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x38048d47               // lea    r8d,[r8+r15*1]
    WORD $0x8941; BYTE $0xdc       // mov    r12d,ebx

    LONG $0x347ffdc5; BYTE $0x24 // vmovdqa [rsp],ymm6
    LONG $0x0f7de3c4; WORD $0x04e3 // vpalignr ymm4,ymm0,ymm3,0x4

    // ROUND(R8, R9, R10, R11, AX, BX, CX, DX, R12, R13, R14, R15, DI, SP, 0xa0)
    LONG $0xa0249403; WORD $0x0000; BYTE $0x00 // add    edx,[rsp+0xa0]
    WORD $0x2141; BYTE $0xc4       // and    r12d,eax
    LONG $0xf07b63c4; WORD $0x19e8 // rorx   r13d,eax,0x19
    LONG $0x0f6de3c4; WORD $0x04f9 // vpalignr ymm7,ymm2,ymm1,0x4
    LONG $0xf07b63c4; WORD $0x0bf8 // rorx   r15d,eax,0xb
    LONG $0x30048d47               // lea    r8d,[r8+r14*1]
    LONG $0x22148d42               // lea    edx,[rdx+r12*1]
    LONG $0xd472cdc5; BYTE $0x07   // vpsrld ymm6,ymm4,0x7
    LONG $0xf27862c4; BYTE $0xe1   // andn   r12d,eax,ecx
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b63c4; WORD $0x06f0 // rorx   r14d,eax,0x6
    LONG $0xdffee5c5               // vpaddd ymm3,ymm3,ymm7
    LONG $0x22148d42               // lea    edx,[rdx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8945; BYTE $0xc7       // mov    r15d,r8d
    LONG $0xd472c5c5; BYTE $0x03   // vpsrld ymm7,ymm4,0x3
    LONG $0xf07b43c4; WORD $0x16e0 // rorx   r12d,r8d,0x16
    LONG $0x2a148d42               // lea    edx,[rdx+r13*1]
    WORD $0x3145; BYTE $0xcf       // xor    r15d,r9d
    LONG $0xf472d5c5; BYTE $0x0e   // vpslld ymm5,ymm4,0xe
    LONG $0xf07b43c4; WORD $0x0df0 // rorx   r14d,r8d,0xd
    LONG $0xf07b43c4; WORD $0x02e8 // rorx   r13d,r8d,0x2
    LONG $0x131c8d45               // lea    r11d,[r11+rdx*1]
    LONG $0xe6efc5c5               // vpxor  ymm4,ymm7,ymm6
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3144; BYTE $0xcf       // xor    edi,r9d
    LONG $0xfa70fdc5; BYTE $0xfa   // vpshufd ymm7,ymm2,0xfa
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    WORD $0x148d; BYTE $0x3a       // lea    edx,[rdx+rdi*1]
    WORD $0x8941; BYTE $0xc4       // mov    r12d,eax
    LONG $0xd672cdc5; BYTE $0x0b   // vpsrld ymm6,ymm6,0xb

    // ROUND(DX, R8, R9, R10, R11, AX, BX, CX, R12, R13, R14, DI, R15, SP, 0xa4)
    LONG $0xa4248c03; WORD $0x0000; BYTE $0x00 // add    ecx,[rsp+0xa4]
    WORD $0x2145; BYTE $0xdc       // and    r12d,r11d
    LONG $0xf07b43c4; WORD $0x19eb // rorx   r13d,r11d,0x19
    LONG $0xe5efddc5               // vpxor  ymm4,ymm4,ymm5
    LONG $0xf07bc3c4; WORD $0x0bfb // rorx   edi,r11d,0xb
    LONG $0x32148d42               // lea    edx,[rdx+r14*1]
    LONG $0x210c8d42               // lea    ecx,[rcx+r12*1]
    LONG $0xf572d5c5; BYTE $0x0b   // vpslld ymm5,ymm5,0xb
    LONG $0xf22062c4; BYTE $0xe3   // andn   r12d,r11d,ebx
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b43c4; WORD $0x06f3 // rorx   r14d,r11d,0x6
    LONG $0xe6efddc5               // vpxor  ymm4,ymm4,ymm6
    LONG $0x210c8d42               // lea    ecx,[rcx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0xd789                   // mov    edi,edx
    LONG $0xd772cdc5; BYTE $0x0a   // vpsrld ymm6,ymm7,0xa
    LONG $0xf07b63c4; WORD $0x16e2 // rorx   r12d,edx,0x16
    LONG $0x290c8d42               // lea    ecx,[rcx+r13*1]
    WORD $0x3144; BYTE $0xc7       // xor    edi,r8d
    LONG $0xe5efddc5               // vpxor  ymm4,ymm4,ymm5
    LONG $0xf07b63c4; WORD $0x0df2 // rorx   r14d,edx,0xd
    LONG $0xf07b63c4; WORD $0x02ea // rorx   r13d,edx,0x2
    LONG $0x0a148d45               // lea    r10d,[r10+rcx*1]
    LONG $0xd773c5c5; BYTE $0x11   // vpsrlq ymm7,ymm7,0x11
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3145; BYTE $0xc7       // xor    r15d,r8d
    LONG $0xdcfee5c5               // vpaddd ymm3,ymm3,ymm4
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x390c8d42               // lea    ecx,[rcx+r15*1]
    WORD $0x8945; BYTE $0xdc       // mov    r12d,r11d
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7

    // ROUND(CX, DX, R8, R9, R10, R11, AX, BX, R12, R13, R14, R15, DI, SP, 0xa8)
    LONG $0xa8249c03; WORD $0x0000; BYTE $0x00 // add    ebx,[rsp+0xa8]
    WORD $0x2145; BYTE $0xd4       // and    r12d,r10d
    LONG $0xf07b43c4; WORD $0x19ea // rorx   r13d,r10d,0x19
    LONG $0xd773c5c5; BYTE $0x02   // vpsrlq ymm7,ymm7,0x2
    LONG $0xf07b43c4; WORD $0x0bfa // rorx   r15d,r10d,0xb
    LONG $0x310c8d42               // lea    ecx,[rcx+r14*1]
    LONG $0x231c8d42               // lea    ebx,[rbx+r12*1]
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7
    LONG $0xf22862c4; BYTE $0xe0   // andn   r12d,r10d,eax
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b43c4; WORD $0x06f2 // rorx   r14d,r10d,0x6
    LONG $0x004dc2c4; BYTE $0xf0   // vpshufb ymm6,ymm6,ymm8
    LONG $0x231c8d42               // lea    ebx,[rbx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8941; BYTE $0xcf       // mov    r15d,ecx
    LONG $0xdefee5c5               // vpaddd ymm3,ymm3,ymm6
    LONG $0xf07b63c4; WORD $0x16e1 // rorx   r12d,ecx,0x16
    LONG $0x2b1c8d42               // lea    ebx,[rbx+r13*1]
    WORD $0x3141; BYTE $0xd7       // xor    r15d,edx
    LONG $0xfb70fdc5; BYTE $0x50   // vpshufd ymm7,ymm3,0x50
    LONG $0xf07b63c4; WORD $0x0df1 // rorx   r14d,ecx,0xd
    LONG $0xf07b63c4; WORD $0x02e9 // rorx   r13d,ecx,0x2
    LONG $0x190c8d45               // lea    r9d,[r9+rbx*1]
    LONG $0xd772cdc5; BYTE $0x0a   // vpsrld ymm6,ymm7,0xa
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0xd731                   // xor    edi,edx
    LONG $0xd773c5c5; BYTE $0x11   // vpsrlq ymm7,ymm7,0x11
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    WORD $0x1c8d; BYTE $0x3b       // lea    ebx,[rbx+rdi*1]
    WORD $0x8945; BYTE $0xd4       // mov    r12d,r10d
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7

    // ROUND(BX, CX, DX, R8, R9, R10, R11, AX, R12, R13, R14, DI, R15, SP, 0xac)
    LONG $0xac248403; WORD $0x0000; BYTE $0x00 // add    eax,[rsp+0xac]
    WORD $0x2145; BYTE $0xcc       // and    r12d,r9d
    LONG $0xf07b43c4; WORD $0x19e9 // rorx   r13d,r9d,0x19
    LONG $0xd773c5c5; BYTE $0x02   // vpsrlq ymm7,ymm7,0x2
    LONG $0xf07bc3c4; WORD $0x0bf9 // rorx   edi,r9d,0xb
    LONG $0x331c8d42               // lea    ebx,[rbx+r14*1]
    LONG $0x20048d42               // lea    eax,[rax+r12*1]
    LONG $0xf7efcdc5               // vpxor  ymm6,ymm6,ymm7
    LONG $0xf23042c4; BYTE $0xe3   // andn   r12d,r9d,r11d
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b43c4; WORD $0x06f1 // rorx   r14d,r9d,0x6
    LONG $0x004dc2c4; BYTE $0xf1   // vpshufb ymm6,ymm6,ymm9
    LONG $0x20048d42               // lea    eax,[rax+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0xdf89                   // mov    edi,ebx
    LONG $0xdefee5c5               // vpaddd ymm3,ymm3,ymm6
    LONG $0xf07b63c4; WORD $0x16e3 // rorx   r12d,ebx,0x16
    LONG $0x28048d42               // lea    eax,[rax+r13*1]
    WORD $0xcf31                   // xor    edi,ecx
    LONG $0x75fee5c5; BYTE $0x60   // vpaddd ymm6,ymm3,[rbp+0x60]
    LONG $0xf07b63c4; WORD $0x0df3 // rorx   r14d,ebx,0xd
    LONG $0xf07b63c4; WORD $0x02eb // rorx   r13d,ebx,0x2
    LONG $0x00048d45               // lea    r8d,[r8+rax*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3141; BYTE $0xcf       // xor    r15d,ecx
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x38048d42               // lea    eax,[rax+r15*1]
    WORD $0x8945; BYTE $0xcc       // mov    r12d,r9d

    LONG $0x747ffdc5; WORD $0x2024 // vmovdqa [rsp+0x20],ymm6
    ADDQ $0x80, BP

    CMPB 0x3(BP),$0x0
    JNE  loop1

    // ROUND(AX, BX, CX, DX, R8, R9, R10, R11, R12, R13, R14, R15, DI, SP, 0x40)
    LONG $0x245c0344; BYTE $0x40   // add    r11d,[rsp+0x40]
    WORD $0x2145; BYTE $0xc4       // and    r12d,r8d
    LONG $0xf07b43c4; WORD $0x19e8 // rorx   r13d,r8d,0x19
    LONG $0xf07b43c4; WORD $0x0bf8 // rorx   r15d,r8d,0xb
    LONG $0x30048d42               // lea    eax,[rax+r14*1]
    LONG $0x231c8d47               // lea    r11d,[r11+r12*1]
    LONG $0xf23842c4; BYTE $0xe2   // andn   r12d,r8d,r10d
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b43c4; WORD $0x06f0 // rorx   r14d,r8d,0x6
    LONG $0x231c8d47               // lea    r11d,[r11+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8941; BYTE $0xc7       // mov    r15d,eax
    LONG $0xf07b63c4; WORD $0x16e0 // rorx   r12d,eax,0x16
    LONG $0x2b1c8d47               // lea    r11d,[r11+r13*1]
    WORD $0x3141; BYTE $0xdf       // xor    r15d,ebx
    LONG $0xf07b63c4; WORD $0x0df0 // rorx   r14d,eax,0xd
    LONG $0xf07b63c4; WORD $0x02e8 // rorx   r13d,eax,0x2
    LONG $0x1a148d42               // lea    edx,[rdx+r11*1]
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0xdf31                   // xor    edi,ebx
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x3b1c8d45               // lea    r11d,[r11+rdi*1]
    WORD $0x8945; BYTE $0xc4       // mov    r12d,r8d

    // ROUND(R11, AX, BX, CX, DX, R8, R9, R10, R12, R13, R14, DI, R15, SP, 0x44)
    LONG $0x24540344; BYTE $0x44   // add    r10d,[rsp+0x44]
    WORD $0x2141; BYTE $0xd4       // and    r12d,edx
    LONG $0xf07b63c4; WORD $0x19ea // rorx   r13d,edx,0x19
    LONG $0xf07be3c4; WORD $0x0bfa // rorx   edi,edx,0xb
    LONG $0x331c8d47               // lea    r11d,[r11+r14*1]
    LONG $0x22148d47               // lea    r10d,[r10+r12*1]
    LONG $0xf26842c4; BYTE $0xe1   // andn   r12d,edx,r9d
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b63c4; WORD $0x06f2 // rorx   r14d,edx,0x6
    LONG $0x22148d47               // lea    r10d,[r10+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8944; BYTE $0xdf       // mov    edi,r11d
    LONG $0xf07b43c4; WORD $0x16e3 // rorx   r12d,r11d,0x16
    LONG $0x2a148d47               // lea    r10d,[r10+r13*1]
    WORD $0xc731                   // xor    edi,eax
    LONG $0xf07b43c4; WORD $0x0df3 // rorx   r14d,r11d,0xd
    LONG $0xf07b43c4; WORD $0x02eb // rorx   r13d,r11d,0x2
    LONG $0x110c8d42               // lea    ecx,[rcx+r10*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3141; BYTE $0xc7       // xor    r15d,eax
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x3a148d47               // lea    r10d,[r10+r15*1]
    WORD $0x8941; BYTE $0xd4       // mov    r12d,edx

    // ROUND(R10, R11, AX, BX, CX, DX, R8, R9, R12, R13, R14, R15, DI, SP, 0x48)
    LONG $0x244c0344; BYTE $0x48   // add    r9d,[rsp+0x48]
    WORD $0x2141; BYTE $0xcc       // and    r12d,ecx
    LONG $0xf07b63c4; WORD $0x19e9 // rorx   r13d,ecx,0x19
    LONG $0xf07b63c4; WORD $0x0bf9 // rorx   r15d,ecx,0xb
    LONG $0x32148d47               // lea    r10d,[r10+r14*1]
    LONG $0x210c8d47               // lea    r9d,[r9+r12*1]
    LONG $0xf27042c4; BYTE $0xe0   // andn   r12d,ecx,r8d
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b63c4; WORD $0x06f1 // rorx   r14d,ecx,0x6
    LONG $0x210c8d47               // lea    r9d,[r9+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8945; BYTE $0xd7       // mov    r15d,r10d
    LONG $0xf07b43c4; WORD $0x16e2 // rorx   r12d,r10d,0x16
    LONG $0x290c8d47               // lea    r9d,[r9+r13*1]
    WORD $0x3145; BYTE $0xdf       // xor    r15d,r11d
    LONG $0xf07b43c4; WORD $0x0df2 // rorx   r14d,r10d,0xd
    LONG $0xf07b43c4; WORD $0x02ea // rorx   r13d,r10d,0x2
    LONG $0x0b1c8d42               // lea    ebx,[rbx+r9*1]
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3144; BYTE $0xdf       // xor    edi,r11d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x390c8d45               // lea    r9d,[r9+rdi*1]
    WORD $0x8941; BYTE $0xcc       // mov    r12d,ecx

    // ROUND(R9, R10, R11, AX, BX, CX, DX, R8, R12, R13, R14, DI, R15, SP, 0x4c)
    LONG $0x24440344; BYTE $0x4c   // add    r8d,[rsp+0x4c]
    WORD $0x2141; BYTE $0xdc       // and    r12d,ebx
    LONG $0xf07b63c4; WORD $0x19eb // rorx   r13d,ebx,0x19
    LONG $0xf07be3c4; WORD $0x0bfb // rorx   edi,ebx,0xb
    LONG $0x310c8d47               // lea    r9d,[r9+r14*1]
    LONG $0x20048d47               // lea    r8d,[r8+r12*1]
    LONG $0xf26062c4; BYTE $0xe2   // andn   r12d,ebx,edx
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b63c4; WORD $0x06f3 // rorx   r14d,ebx,0x6
    LONG $0x20048d47               // lea    r8d,[r8+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8944; BYTE $0xcf       // mov    edi,r9d
    LONG $0xf07b43c4; WORD $0x16e1 // rorx   r12d,r9d,0x16
    LONG $0x28048d47               // lea    r8d,[r8+r13*1]
    WORD $0x3144; BYTE $0xd7       // xor    edi,r10d
    LONG $0xf07b43c4; WORD $0x0df1 // rorx   r14d,r9d,0xd
    LONG $0xf07b43c4; WORD $0x02e9 // rorx   r13d,r9d,0x2
    LONG $0x00048d42               // lea    eax,[rax+r8*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3145; BYTE $0xd7       // xor    r15d,r10d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x38048d47               // lea    r8d,[r8+r15*1]
    WORD $0x8941; BYTE $0xdc       // mov    r12d,ebx

    // ROUND(R8, R9, R10, R11, AX, BX, CX, DX, R12, R13, R14, R15, DI, SP, 0x60)
    LONG $0x60245403               // add    edx,[rsp+0x60]
    WORD $0x2141; BYTE $0xc4       // and    r12d,eax
    LONG $0xf07b63c4; WORD $0x19e8 // rorx   r13d,eax,0x19
    LONG $0xf07b63c4; WORD $0x0bf8 // rorx   r15d,eax,0xb
    LONG $0x30048d47               // lea    r8d,[r8+r14*1]
    LONG $0x22148d42               // lea    edx,[rdx+r12*1]
    LONG $0xf27862c4; BYTE $0xe1   // andn   r12d,eax,ecx
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b63c4; WORD $0x06f0 // rorx   r14d,eax,0x6
    LONG $0x22148d42               // lea    edx,[rdx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8945; BYTE $0xc7       // mov    r15d,r8d
    LONG $0xf07b43c4; WORD $0x16e0 // rorx   r12d,r8d,0x16
    LONG $0x2a148d42               // lea    edx,[rdx+r13*1]
    WORD $0x3145; BYTE $0xcf       // xor    r15d,r9d
    LONG $0xf07b43c4; WORD $0x0df0 // rorx   r14d,r8d,0xd
    LONG $0xf07b43c4; WORD $0x02e8 // rorx   r13d,r8d,0x2
    LONG $0x131c8d45               // lea    r11d,[r11+rdx*1]
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3144; BYTE $0xcf       // xor    edi,r9d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    WORD $0x148d; BYTE $0x3a       // lea    edx,[rdx+rdi*1]
    WORD $0x8941; BYTE $0xc4       // mov    r12d,eax

    // ROUND(DX, R8, R9, R10, R11, AX, BX, CX, R12, R13, R14, DI, R15, SP, 0x64)
    LONG $0x64244c03               // add    ecx,[rsp+0x64]
    WORD $0x2145; BYTE $0xdc       // and    r12d,r11d
    LONG $0xf07b43c4; WORD $0x19eb // rorx   r13d,r11d,0x19
    LONG $0xf07bc3c4; WORD $0x0bfb // rorx   edi,r11d,0xb
    LONG $0x32148d42               // lea    edx,[rdx+r14*1]
    LONG $0x210c8d42               // lea    ecx,[rcx+r12*1]
    LONG $0xf22062c4; BYTE $0xe3   // andn   r12d,r11d,ebx
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b43c4; WORD $0x06f3 // rorx   r14d,r11d,0x6
    LONG $0x210c8d42               // lea    ecx,[rcx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0xd789                   // mov    edi,edx
    LONG $0xf07b63c4; WORD $0x16e2 // rorx   r12d,edx,0x16
    LONG $0x290c8d42               // lea    ecx,[rcx+r13*1]
    WORD $0x3144; BYTE $0xc7       // xor    edi,r8d
    LONG $0xf07b63c4; WORD $0x0df2 // rorx   r14d,edx,0xd
    LONG $0xf07b63c4; WORD $0x02ea // rorx   r13d,edx,0x2
    LONG $0x0a148d45               // lea    r10d,[r10+rcx*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3145; BYTE $0xc7       // xor    r15d,r8d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x390c8d42               // lea    ecx,[rcx+r15*1]
    WORD $0x8945; BYTE $0xdc       // mov    r12d,r11d

    // ROUND(CX, DX, R8, R9, R10, R11, AX, BX, R12, R13, R14, R15, DI, SP, 0x68)
    LONG $0x68245c03               // add    ebx,[rsp+0x68]
    WORD $0x2145; BYTE $0xd4       // and    r12d,r10d
    LONG $0xf07b43c4; WORD $0x19ea // rorx   r13d,r10d,0x19
    LONG $0xf07b43c4; WORD $0x0bfa // rorx   r15d,r10d,0xb
    LONG $0x310c8d42               // lea    ecx,[rcx+r14*1]
    LONG $0x231c8d42               // lea    ebx,[rbx+r12*1]
    LONG $0xf22862c4; BYTE $0xe0   // andn   r12d,r10d,eax
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b43c4; WORD $0x06f2 // rorx   r14d,r10d,0x6
    LONG $0x231c8d42               // lea    ebx,[rbx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8941; BYTE $0xcf       // mov    r15d,ecx
    LONG $0xf07b63c4; WORD $0x16e1 // rorx   r12d,ecx,0x16
    LONG $0x2b1c8d42               // lea    ebx,[rbx+r13*1]
    WORD $0x3141; BYTE $0xd7       // xor    r15d,edx
    LONG $0xf07b63c4; WORD $0x0df1 // rorx   r14d,ecx,0xd
    LONG $0xf07b63c4; WORD $0x02e9 // rorx   r13d,ecx,0x2
    LONG $0x190c8d45               // lea    r9d,[r9+rbx*1]
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0xd731                   // xor    edi,edx
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    WORD $0x1c8d; BYTE $0x3b       // lea    ebx,[rbx+rdi*1]
    WORD $0x8945; BYTE $0xd4       // mov    r12d,r10d

    // ROUND(BX, CX, DX, R8, R9, R10, R11, AX, R12, R13, R14, DI, R15, SP, 0x6c)
    LONG $0x6c244403               // add    eax,[rsp+0x6c]
    WORD $0x2145; BYTE $0xcc       // and    r12d,r9d
    LONG $0xf07b43c4; WORD $0x19e9 // rorx   r13d,r9d,0x19
    LONG $0xf07bc3c4; WORD $0x0bf9 // rorx   edi,r9d,0xb
    LONG $0x331c8d42               // lea    ebx,[rbx+r14*1]
    LONG $0x20048d42               // lea    eax,[rax+r12*1]
    LONG $0xf23042c4; BYTE $0xe3   // andn   r12d,r9d,r11d
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b43c4; WORD $0x06f1 // rorx   r14d,r9d,0x6
    LONG $0x20048d42               // lea    eax,[rax+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0xdf89                   // mov    edi,ebx
    LONG $0xf07b63c4; WORD $0x16e3 // rorx   r12d,ebx,0x16
    LONG $0x28048d42               // lea    eax,[rax+r13*1]
    WORD $0xcf31                   // xor    edi,ecx
    LONG $0xf07b63c4; WORD $0x0df3 // rorx   r14d,ebx,0xd
    LONG $0xf07b63c4; WORD $0x02eb // rorx   r13d,ebx,0x2
    LONG $0x00048d45               // lea    r8d,[r8+rax*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3141; BYTE $0xcf       // xor    r15d,ecx
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x38048d42               // lea    eax,[rax+r15*1]
    WORD $0x8945; BYTE $0xcc       // mov    r12d,r9d

    // ROUND(AX, BX, CX, DX, R8, R9, R10, R11, R12, R13, R14, R15, DI, SP, 0x00)
    LONG $0x241c0344               // add    r11d,[rsp]
    WORD $0x2145; BYTE $0xc4       // and    r12d,r8d
    LONG $0xf07b43c4; WORD $0x19e8 // rorx   r13d,r8d,0x19
    LONG $0xf07b43c4; WORD $0x0bf8 // rorx   r15d,r8d,0xb
    LONG $0x30048d42               // lea    eax,[rax+r14*1]
    LONG $0x231c8d47               // lea    r11d,[r11+r12*1]
    LONG $0xf23842c4; BYTE $0xe2   // andn   r12d,r8d,r10d
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b43c4; WORD $0x06f0 // rorx   r14d,r8d,0x6
    LONG $0x231c8d47               // lea    r11d,[r11+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8941; BYTE $0xc7       // mov    r15d,eax
    LONG $0xf07b63c4; WORD $0x16e0 // rorx   r12d,eax,0x16
    LONG $0x2b1c8d47               // lea    r11d,[r11+r13*1]
    WORD $0x3141; BYTE $0xdf       // xor    r15d,ebx
    LONG $0xf07b63c4; WORD $0x0df0 // rorx   r14d,eax,0xd
    LONG $0xf07b63c4; WORD $0x02e8 // rorx   r13d,eax,0x2
    LONG $0x1a148d42               // lea    edx,[rdx+r11*1]
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0xdf31                   // xor    edi,ebx
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x3b1c8d45               // lea    r11d,[r11+rdi*1]
    WORD $0x8945; BYTE $0xc4       // mov    r12d,r8d

    // ROUND(R11, AX, BX, CX, DX, R8, R9, R10, R12, R13, R14, DI, R15, SP, 0x04)
    LONG $0x24540344; BYTE $0x04   // add    r10d,[rsp+0x4]
    WORD $0x2141; BYTE $0xd4       // and    r12d,edx
    LONG $0xf07b63c4; WORD $0x19ea // rorx   r13d,edx,0x19
    LONG $0xf07be3c4; WORD $0x0bfa // rorx   edi,edx,0xb
    LONG $0x331c8d47               // lea    r11d,[r11+r14*1]
    LONG $0x22148d47               // lea    r10d,[r10+r12*1]
    LONG $0xf26842c4; BYTE $0xe1   // andn   r12d,edx,r9d
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b63c4; WORD $0x06f2 // rorx   r14d,edx,0x6
    LONG $0x22148d47               // lea    r10d,[r10+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8944; BYTE $0xdf       // mov    edi,r11d
    LONG $0xf07b43c4; WORD $0x16e3 // rorx   r12d,r11d,0x16
    LONG $0x2a148d47               // lea    r10d,[r10+r13*1]
    WORD $0xc731                   // xor    edi,eax
    LONG $0xf07b43c4; WORD $0x0df3 // rorx   r14d,r11d,0xd
    LONG $0xf07b43c4; WORD $0x02eb // rorx   r13d,r11d,0x2
    LONG $0x110c8d42               // lea    ecx,[rcx+r10*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3141; BYTE $0xc7       // xor    r15d,eax
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x3a148d47               // lea    r10d,[r10+r15*1]
    WORD $0x8941; BYTE $0xd4       // mov    r12d,edx

    // ROUND(R10, R11, AX, BX, CX, DX, R8, R9, R12, R13, R14, R15, DI, SP, 0x08)
    LONG $0x244c0344; BYTE $0x08   // add    r9d,[rsp+0x8]
    WORD $0x2141; BYTE $0xcc       // and    r12d,ecx
    LONG $0xf07b63c4; WORD $0x19e9 // rorx   r13d,ecx,0x19
    LONG $0xf07b63c4; WORD $0x0bf9 // rorx   r15d,ecx,0xb
    LONG $0x32148d47               // lea    r10d,[r10+r14*1]
    LONG $0x210c8d47               // lea    r9d,[r9+r12*1]
    LONG $0xf27042c4; BYTE $0xe0   // andn   r12d,ecx,r8d
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b63c4; WORD $0x06f1 // rorx   r14d,ecx,0x6
    LONG $0x210c8d47               // lea    r9d,[r9+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8945; BYTE $0xd7       // mov    r15d,r10d
    LONG $0xf07b43c4; WORD $0x16e2 // rorx   r12d,r10d,0x16
    LONG $0x290c8d47               // lea    r9d,[r9+r13*1]
    WORD $0x3145; BYTE $0xdf       // xor    r15d,r11d
    LONG $0xf07b43c4; WORD $0x0df2 // rorx   r14d,r10d,0xd
    LONG $0xf07b43c4; WORD $0x02ea // rorx   r13d,r10d,0x2
    LONG $0x0b1c8d42               // lea    ebx,[rbx+r9*1]
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3144; BYTE $0xdf       // xor    edi,r11d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x390c8d45               // lea    r9d,[r9+rdi*1]
    WORD $0x8941; BYTE $0xcc       // mov    r12d,ecx

    // ROUND(R9, R10, R11, AX, BX, CX, DX, R8, R12, R13, R14, DI, R15, SP, 0x0c)
    LONG $0x24440344; BYTE $0x0c   // add    r8d,[rsp+0xc]
    WORD $0x2141; BYTE $0xdc       // and    r12d,ebx
    LONG $0xf07b63c4; WORD $0x19eb // rorx   r13d,ebx,0x19
    LONG $0xf07be3c4; WORD $0x0bfb // rorx   edi,ebx,0xb
    LONG $0x310c8d47               // lea    r9d,[r9+r14*1]
    LONG $0x20048d47               // lea    r8d,[r8+r12*1]
    LONG $0xf26062c4; BYTE $0xe2   // andn   r12d,ebx,edx
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b63c4; WORD $0x06f3 // rorx   r14d,ebx,0x6
    LONG $0x20048d47               // lea    r8d,[r8+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8944; BYTE $0xcf       // mov    edi,r9d
    LONG $0xf07b43c4; WORD $0x16e1 // rorx   r12d,r9d,0x16
    LONG $0x28048d47               // lea    r8d,[r8+r13*1]
    WORD $0x3144; BYTE $0xd7       // xor    edi,r10d
    LONG $0xf07b43c4; WORD $0x0df1 // rorx   r14d,r9d,0xd
    LONG $0xf07b43c4; WORD $0x02e9 // rorx   r13d,r9d,0x2
    LONG $0x00048d42               // lea    eax,[rax+r8*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3145; BYTE $0xd7       // xor    r15d,r10d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x38048d47               // lea    r8d,[r8+r15*1]
    WORD $0x8941; BYTE $0xdc       // mov    r12d,ebx

    // ROUND(R8, R9, R10, R11, AX, BX, CX, DX, R12, R13, R14, R15, DI, SP, 0x20)
    LONG $0x20245403               // add    edx,[rsp+0x20]
    WORD $0x2141; BYTE $0xc4       // and    r12d,eax
    LONG $0xf07b63c4; WORD $0x19e8 // rorx   r13d,eax,0x19
    LONG $0xf07b63c4; WORD $0x0bf8 // rorx   r15d,eax,0xb
    LONG $0x30048d47               // lea    r8d,[r8+r14*1]
    LONG $0x22148d42               // lea    edx,[rdx+r12*1]
    LONG $0xf27862c4; BYTE $0xe1   // andn   r12d,eax,ecx
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b63c4; WORD $0x06f0 // rorx   r14d,eax,0x6
    LONG $0x22148d42               // lea    edx,[rdx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8945; BYTE $0xc7       // mov    r15d,r8d
    LONG $0xf07b43c4; WORD $0x16e0 // rorx   r12d,r8d,0x16
    LONG $0x2a148d42               // lea    edx,[rdx+r13*1]
    WORD $0x3145; BYTE $0xcf       // xor    r15d,r9d
    LONG $0xf07b43c4; WORD $0x0df0 // rorx   r14d,r8d,0xd
    LONG $0xf07b43c4; WORD $0x02e8 // rorx   r13d,r8d,0x2
    LONG $0x131c8d45               // lea    r11d,[r11+rdx*1]
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3144; BYTE $0xcf       // xor    edi,r9d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    WORD $0x148d; BYTE $0x3a       // lea    edx,[rdx+rdi*1]
    WORD $0x8941; BYTE $0xc4       // mov    r12d,eax

    // ROUND(DX, R8, R9, R10, R11, AX, BX, CX, R12, R13, R14, DI, R15, SP, 0x24)
    LONG $0x24244c03               // add    ecx,[rsp+0x24]
    WORD $0x2145; BYTE $0xdc       // and    r12d,r11d
    LONG $0xf07b43c4; WORD $0x19eb // rorx   r13d,r11d,0x19
    LONG $0xf07bc3c4; WORD $0x0bfb // rorx   edi,r11d,0xb
    LONG $0x32148d42               // lea    edx,[rdx+r14*1]
    LONG $0x210c8d42               // lea    ecx,[rcx+r12*1]
    LONG $0xf22062c4; BYTE $0xe3   // andn   r12d,r11d,ebx
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b43c4; WORD $0x06f3 // rorx   r14d,r11d,0x6
    LONG $0x210c8d42               // lea    ecx,[rcx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0xd789                   // mov    edi,edx
    LONG $0xf07b63c4; WORD $0x16e2 // rorx   r12d,edx,0x16
    LONG $0x290c8d42               // lea    ecx,[rcx+r13*1]
    WORD $0x3144; BYTE $0xc7       // xor    edi,r8d
    LONG $0xf07b63c4; WORD $0x0df2 // rorx   r14d,edx,0xd
    LONG $0xf07b63c4; WORD $0x02ea // rorx   r13d,edx,0x2
    LONG $0x0a148d45               // lea    r10d,[r10+rcx*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3145; BYTE $0xc7       // xor    r15d,r8d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x390c8d42               // lea    ecx,[rcx+r15*1]
    WORD $0x8945; BYTE $0xdc       // mov    r12d,r11d

    // ROUND(CX, DX, R8, R9, R10, R11, AX, BX, R12, R13, R14, R15, DI, SP, 0x28)
    LONG $0x28245c03               // add    ebx,[rsp+0x28]
    WORD $0x2145; BYTE $0xd4       // and    r12d,r10d
    LONG $0xf07b43c4; WORD $0x19ea // rorx   r13d,r10d,0x19
    LONG $0xf07b43c4; WORD $0x0bfa // rorx   r15d,r10d,0xb
    LONG $0x310c8d42               // lea    ecx,[rcx+r14*1]
    LONG $0x231c8d42               // lea    ebx,[rbx+r12*1]
    LONG $0xf22862c4; BYTE $0xe0   // andn   r12d,r10d,eax
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b43c4; WORD $0x06f2 // rorx   r14d,r10d,0x6
    LONG $0x231c8d42               // lea    ebx,[rbx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8941; BYTE $0xcf       // mov    r15d,ecx
    LONG $0xf07b63c4; WORD $0x16e1 // rorx   r12d,ecx,0x16
    LONG $0x2b1c8d42               // lea    ebx,[rbx+r13*1]
    WORD $0x3141; BYTE $0xd7       // xor    r15d,edx
    LONG $0xf07b63c4; WORD $0x0df1 // rorx   r14d,ecx,0xd
    LONG $0xf07b63c4; WORD $0x02e9 // rorx   r13d,ecx,0x2
    LONG $0x190c8d45               // lea    r9d,[r9+rbx*1]
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0xd731                   // xor    edi,edx
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    WORD $0x1c8d; BYTE $0x3b       // lea    ebx,[rbx+rdi*1]
    WORD $0x8945; BYTE $0xd4       // mov    r12d,r10d

    // ROUND(BX, CX, DX, R8, R9, R10, R11, AX, R12, R13, R14, DI, R15, SP, 0x2c)
    LONG $0x2c244403               // add    eax,[rsp+0x2c]
    WORD $0x2145; BYTE $0xcc       // and    r12d,r9d
    LONG $0xf07b43c4; WORD $0x19e9 // rorx   r13d,r9d,0x19
    LONG $0xf07bc3c4; WORD $0x0bf9 // rorx   edi,r9d,0xb
    LONG $0x331c8d42               // lea    ebx,[rbx+r14*1]
    LONG $0x20048d42               // lea    eax,[rax+r12*1]
    LONG $0xf23042c4; BYTE $0xe3   // andn   r12d,r9d,r11d
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b43c4; WORD $0x06f1 // rorx   r14d,r9d,0x6
    LONG $0x20048d42               // lea    eax,[rax+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0xdf89                   // mov    edi,ebx
    LONG $0xf07b63c4; WORD $0x16e3 // rorx   r12d,ebx,0x16
    LONG $0x28048d42               // lea    eax,[rax+r13*1]
    WORD $0xcf31                   // xor    edi,ecx
    LONG $0xf07b63c4; WORD $0x0df3 // rorx   r14d,ebx,0xd
    LONG $0xf07b63c4; WORD $0x02eb // rorx   r13d,ebx,0x2
    LONG $0x00048d45               // lea    r8d,[r8+rax*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3141; BYTE $0xcf       // xor    r15d,ecx
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x38048d42               // lea    eax,[rax+r15*1]
    WORD $0x8945; BYTE $0xcc       // mov    r12d,r9d

    MOVQ 0x200(SP), DI             // $_ctx
    ADDQ R14, AX

    LEAQ 0x1c0(SP), BP

    ADDL (DI), AX
    ADDL 4(DI), BX
    ADDL 8(DI), CX
    ADDL 12(DI), DX
    ADDL 16(DI), R8
    ADDL 20(DI), R9
    ADDL 24(DI), R10
    ADDL 28(DI), R11

    MOVL AX, (DI)
    MOVL BX, 4(DI)
    MOVL CX, 8(DI)
    MOVL DX, 12(DI)
    MOVL R8, 16(DI)
    MOVL R9, 20(DI)
    MOVL R10, 24(DI)
    MOVL R11, 28(DI)

    CMPQ SI, 0x50(BP)              // $_end
    JE   done

    XORQ R14, R14
    MOVQ BX, DI
    XORQ CX, DI                    // magic
    MOVQ R9, R12

loop2:
    // ROUND(AX, BX, CX, DX, R8, R9, R10, R11, R12, R13, R14, R15, DI, BP, 0x10)
    LONG $0x105d0344               // add    r11d,[rbp+0x10]
    WORD $0x2145; BYTE $0xc4       // and    r12d,r8d
    LONG $0xf07b43c4; WORD $0x19e8 // rorx   r13d,r8d,0x19
    LONG $0xf07b43c4; WORD $0x0bf8 // rorx   r15d,r8d,0xb
    LONG $0x30048d42               // lea    eax,[rax+r14*1]
    LONG $0x231c8d47               // lea    r11d,[r11+r12*1]
    LONG $0xf23842c4; BYTE $0xe2   // andn   r12d,r8d,r10d
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b43c4; WORD $0x06f0 // rorx   r14d,r8d,0x6
    LONG $0x231c8d47               // lea    r11d,[r11+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8941; BYTE $0xc7       // mov    r15d,eax
    LONG $0xf07b63c4; WORD $0x16e0 // rorx   r12d,eax,0x16
    LONG $0x2b1c8d47               // lea    r11d,[r11+r13*1]
    WORD $0x3141; BYTE $0xdf       // xor    r15d,ebx
    LONG $0xf07b63c4; WORD $0x0df0 // rorx   r14d,eax,0xd
    LONG $0xf07b63c4; WORD $0x02e8 // rorx   r13d,eax,0x2
    LONG $0x1a148d42               // lea    edx,[rdx+r11*1]
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0xdf31                   // xor    edi,ebx
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x3b1c8d45               // lea    r11d,[r11+rdi*1]
    WORD $0x8945; BYTE $0xc4       // mov    r12d,r8d

    // ROUND(R11, AX, BX, CX, DX, R8, R9, R10, R12, R13, R14, DI, R15, BP, 0x14)
    LONG $0x14550344               // add    r10d,[rbp+0x14]
    WORD $0x2141; BYTE $0xd4       // and    r12d,edx
    LONG $0xf07b63c4; WORD $0x19ea // rorx   r13d,edx,0x19
    LONG $0xf07be3c4; WORD $0x0bfa // rorx   edi,edx,0xb
    LONG $0x331c8d47               // lea    r11d,[r11+r14*1]
    LONG $0x22148d47               // lea    r10d,[r10+r12*1]
    LONG $0xf26842c4; BYTE $0xe1   // andn   r12d,edx,r9d
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b63c4; WORD $0x06f2 // rorx   r14d,edx,0x6
    LONG $0x22148d47               // lea    r10d,[r10+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8944; BYTE $0xdf       // mov    edi,r11d
    LONG $0xf07b43c4; WORD $0x16e3 // rorx   r12d,r11d,0x16
    LONG $0x2a148d47               // lea    r10d,[r10+r13*1]
    WORD $0xc731                   // xor    edi,eax
    LONG $0xf07b43c4; WORD $0x0df3 // rorx   r14d,r11d,0xd
    LONG $0xf07b43c4; WORD $0x02eb // rorx   r13d,r11d,0x2
    LONG $0x110c8d42               // lea    ecx,[rcx+r10*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3141; BYTE $0xc7       // xor    r15d,eax
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x3a148d47               // lea    r10d,[r10+r15*1]
    WORD $0x8941; BYTE $0xd4       // mov    r12d,edx

    // ROUND(R10, R11, AX, BX, CX, DX, R8, R9, R12, R13, R14, R15, DI, BP, 0x18)
    LONG $0x184d0344               // add    r9d,[rbp+0x18]
    WORD $0x2141; BYTE $0xcc       // and    r12d,ecx
    LONG $0xf07b63c4; WORD $0x19e9 // rorx   r13d,ecx,0x19
    LONG $0xf07b63c4; WORD $0x0bf9 // rorx   r15d,ecx,0xb
    LONG $0x32148d47               // lea    r10d,[r10+r14*1]
    LONG $0x210c8d47               // lea    r9d,[r9+r12*1]
    LONG $0xf27042c4; BYTE $0xe0   // andn   r12d,ecx,r8d
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b63c4; WORD $0x06f1 // rorx   r14d,ecx,0x6
    LONG $0x210c8d47               // lea    r9d,[r9+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8945; BYTE $0xd7       // mov    r15d,r10d
    LONG $0xf07b43c4; WORD $0x16e2 // rorx   r12d,r10d,0x16
    LONG $0x290c8d47               // lea    r9d,[r9+r13*1]
    WORD $0x3145; BYTE $0xdf       // xor    r15d,r11d
    LONG $0xf07b43c4; WORD $0x0df2 // rorx   r14d,r10d,0xd
    LONG $0xf07b43c4; WORD $0x02ea // rorx   r13d,r10d,0x2
    LONG $0x0b1c8d42               // lea    ebx,[rbx+r9*1]
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3144; BYTE $0xdf       // xor    edi,r11d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x390c8d45               // lea    r9d,[r9+rdi*1]
    WORD $0x8941; BYTE $0xcc       // mov    r12d,ecx

    // ROUND(R9, R10, R11, AX, BX, CX, DX, R8, R12, R13, R14, DI, R15, BP, 0x1c)
    LONG $0x1c450344               // add    r8d,[rbp+0x1c]
    WORD $0x2141; BYTE $0xdc       // and    r12d,ebx
    LONG $0xf07b63c4; WORD $0x19eb // rorx   r13d,ebx,0x19
    LONG $0xf07be3c4; WORD $0x0bfb // rorx   edi,ebx,0xb
    LONG $0x310c8d47               // lea    r9d,[r9+r14*1]
    LONG $0x20048d47               // lea    r8d,[r8+r12*1]
    LONG $0xf26062c4; BYTE $0xe2   // andn   r12d,ebx,edx
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b63c4; WORD $0x06f3 // rorx   r14d,ebx,0x6
    LONG $0x20048d47               // lea    r8d,[r8+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8944; BYTE $0xcf       // mov    edi,r9d
    LONG $0xf07b43c4; WORD $0x16e1 // rorx   r12d,r9d,0x16
    LONG $0x28048d47               // lea    r8d,[r8+r13*1]
    WORD $0x3144; BYTE $0xd7       // xor    edi,r10d
    LONG $0xf07b43c4; WORD $0x0df1 // rorx   r14d,r9d,0xd
    LONG $0xf07b43c4; WORD $0x02e9 // rorx   r13d,r9d,0x2
    LONG $0x00048d42               // lea    eax,[rax+r8*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3145; BYTE $0xd7       // xor    r15d,r10d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x38048d47               // lea    r8d,[r8+r15*1]
    WORD $0x8941; BYTE $0xdc       // mov    r12d,ebx

    // ROUND(R8, R9, R10, R11, AX, BX, CX, DX, R12, R13, R14, R15, DI, BP, 0x30)
    WORD $0x5503; BYTE $0x30       // add    edx,[rbp+0x30]
    WORD $0x2141; BYTE $0xc4       // and    r12d,eax
    LONG $0xf07b63c4; WORD $0x19e8 // rorx   r13d,eax,0x19
    LONG $0xf07b63c4; WORD $0x0bf8 // rorx   r15d,eax,0xb
    LONG $0x30048d47               // lea    r8d,[r8+r14*1]
    LONG $0x22148d42               // lea    edx,[rdx+r12*1]
    LONG $0xf27862c4; BYTE $0xe1   // andn   r12d,eax,ecx
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b63c4; WORD $0x06f0 // rorx   r14d,eax,0x6
    LONG $0x22148d42               // lea    edx,[rdx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8945; BYTE $0xc7       // mov    r15d,r8d
    LONG $0xf07b43c4; WORD $0x16e0 // rorx   r12d,r8d,0x16
    LONG $0x2a148d42               // lea    edx,[rdx+r13*1]
    WORD $0x3145; BYTE $0xcf       // xor    r15d,r9d
    LONG $0xf07b43c4; WORD $0x0df0 // rorx   r14d,r8d,0xd
    LONG $0xf07b43c4; WORD $0x02e8 // rorx   r13d,r8d,0x2
    LONG $0x131c8d45               // lea    r11d,[r11+rdx*1]
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3144; BYTE $0xcf       // xor    edi,r9d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    WORD $0x148d; BYTE $0x3a       // lea    edx,[rdx+rdi*1]
    WORD $0x8941; BYTE $0xc4       // mov    r12d,eax

    // ROUND(DX, R8, R9, R10, R11, AX, BX, CX, R12, R13, R14, DI, R15, BP, 0x34)
    WORD $0x4d03; BYTE $0x34       // add    ecx,[rbp+0x34]
    WORD $0x2145; BYTE $0xdc       // and    r12d,r11d
    LONG $0xf07b43c4; WORD $0x19eb // rorx   r13d,r11d,0x19
    LONG $0xf07bc3c4; WORD $0x0bfb // rorx   edi,r11d,0xb
    LONG $0x32148d42               // lea    edx,[rdx+r14*1]
    LONG $0x210c8d42               // lea    ecx,[rcx+r12*1]
    LONG $0xf22062c4; BYTE $0xe3   // andn   r12d,r11d,ebx
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b43c4; WORD $0x06f3 // rorx   r14d,r11d,0x6
    LONG $0x210c8d42               // lea    ecx,[rcx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0xd789                   // mov    edi,edx
    LONG $0xf07b63c4; WORD $0x16e2 // rorx   r12d,edx,0x16
    LONG $0x290c8d42               // lea    ecx,[rcx+r13*1]
    WORD $0x3144; BYTE $0xc7       // xor    edi,r8d
    LONG $0xf07b63c4; WORD $0x0df2 // rorx   r14d,edx,0xd
    LONG $0xf07b63c4; WORD $0x02ea // rorx   r13d,edx,0x2
    LONG $0x0a148d45               // lea    r10d,[r10+rcx*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3145; BYTE $0xc7       // xor    r15d,r8d
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x390c8d42               // lea    ecx,[rcx+r15*1]
    WORD $0x8945; BYTE $0xdc       // mov    r12d,r11d

    // ROUND(CX, DX, R8, R9, R10, R11, AX, BX, R12, R13, R14, R15, DI, BP, 0x38)
    WORD $0x5d03; BYTE $0x38       // add    ebx,[rbp+0x38]
    WORD $0x2145; BYTE $0xd4       // and    r12d,r10d
    LONG $0xf07b43c4; WORD $0x19ea // rorx   r13d,r10d,0x19
    LONG $0xf07b43c4; WORD $0x0bfa // rorx   r15d,r10d,0xb
    LONG $0x310c8d42               // lea    ecx,[rcx+r14*1]
    LONG $0x231c8d42               // lea    ebx,[rbx+r12*1]
    LONG $0xf22862c4; BYTE $0xe0   // andn   r12d,r10d,eax
    WORD $0x3145; BYTE $0xfd       // xor    r13d,r15d
    LONG $0xf07b43c4; WORD $0x06f2 // rorx   r14d,r10d,0x6
    LONG $0x231c8d42               // lea    ebx,[rbx+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0x8941; BYTE $0xcf       // mov    r15d,ecx
    LONG $0xf07b63c4; WORD $0x16e1 // rorx   r12d,ecx,0x16
    LONG $0x2b1c8d42               // lea    ebx,[rbx+r13*1]
    WORD $0x3141; BYTE $0xd7       // xor    r15d,edx
    LONG $0xf07b63c4; WORD $0x0df1 // rorx   r14d,ecx,0xd
    LONG $0xf07b63c4; WORD $0x02e9 // rorx   r13d,ecx,0x2
    LONG $0x190c8d45               // lea    r9d,[r9+rbx*1]
    WORD $0x2144; BYTE $0xff       // and    edi,r15d
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0xd731                   // xor    edi,edx
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    WORD $0x1c8d; BYTE $0x3b       // lea    ebx,[rbx+rdi*1]
    WORD $0x8945; BYTE $0xd4       // mov    r12d,r10d

    // ROUND(BX, CX, DX, R8, R9, R10, R11, AX, R12, R13, R14, DI, R15, BP, 0x3c)
    WORD $0x4503; BYTE $0x3c       // add    eax,[rbp+0x3c]
    WORD $0x2145; BYTE $0xcc       // and    r12d,r9d
    LONG $0xf07b43c4; WORD $0x19e9 // rorx   r13d,r9d,0x19
    LONG $0xf07bc3c4; WORD $0x0bf9 // rorx   edi,r9d,0xb
    LONG $0x331c8d42               // lea    ebx,[rbx+r14*1]
    LONG $0x20048d42               // lea    eax,[rax+r12*1]
    LONG $0xf23042c4; BYTE $0xe3   // andn   r12d,r9d,r11d
    WORD $0x3141; BYTE $0xfd       // xor    r13d,edi
    LONG $0xf07b43c4; WORD $0x06f1 // rorx   r14d,r9d,0x6
    LONG $0x20048d42               // lea    eax,[rax+r12*1]
    WORD $0x3145; BYTE $0xf5       // xor    r13d,r14d
    WORD $0xdf89                   // mov    edi,ebx
    LONG $0xf07b63c4; WORD $0x16e3 // rorx   r12d,ebx,0x16
    LONG $0x28048d42               // lea    eax,[rax+r13*1]
    WORD $0xcf31                   // xor    edi,ecx
    LONG $0xf07b63c4; WORD $0x0df3 // rorx   r14d,ebx,0xd
    LONG $0xf07b63c4; WORD $0x02eb // rorx   r13d,ebx,0x2
    LONG $0x00048d45               // lea    r8d,[r8+rax*1]
    WORD $0x2141; BYTE $0xff       // and    r15d,edi
    WORD $0x3145; BYTE $0xe6       // xor    r14d,r12d
    WORD $0x3141; BYTE $0xcf       // xor    r15d,ecx
    WORD $0x3145; BYTE $0xee       // xor    r14d,r13d
    LONG $0x38048d42               // lea    eax,[rax+r15*1]
    WORD $0x8945; BYTE $0xcc       // mov    r12d,r9d

    ADDQ $-0x40, BP
    CMPQ BP, SP
    JAE  loop2

    MOVQ 0x200(SP), DI             // $_ctx
    ADDQ R14, AX

    ADDQ $0x1c0, SP

    ADDL (DI), AX
    ADDL 4(DI), BX
    ADDL 8(DI), CX
    ADDL 12(DI), DX
    ADDL 16(DI), R8
    ADDL 20(DI), R9

    ADDQ $0x80, SI                 // input += 2
    ADDL 24(DI), R10
    MOVQ SI, R12
    ADDL 28(DI), R11
    CMPQ  SI, 0x50(SP)             // input == _end

    MOVL AX, (DI)
    LONG $0xe4440f4c               // cmove  r12,rsp                /* next block or stale data */
    MOVL AX, (DI)
    MOVL BX, 4(DI)
    MOVL CX, 8(DI)
    MOVL DX, 12(DI)
    MOVL R8, 16(DI)
    MOVL R9, 20(DI)
    MOVL R10, 24(DI)
    MOVL R11, 28(DI)

    JBE loop0
    LEAQ (SP), BP

done:
    MOVQ BP, SP
    MOVQ 0x58(SP), SP
    WORD $0xf8c5; BYTE $0x77     // vzeroupper

    RET

