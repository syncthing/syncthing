//+build !noasm !appengine

// SHA256 implementation for SSSE3

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

#define ROTATE_XS \
	MOVOU X4, X15 \
	MOVOU X5, X4  \
	MOVOU X6, X5  \
	MOVOU X7, X6  \
	MOVOU X15, X7

// compute s0 four at a time and s1 two at a time
// compute W[-16] + W[-7] 4 at a time
#define FOUR_ROUNDS_AND_SCHED(a, b, c, d, e, f, g, h) \
	MOVL  e, R13                    \ // y0 = e
	ROLL  $18, R13                  \ // y0 = e >> (25-11)
	MOVL  a, R14                    \ // y1 = a
	MOVOU X7, X0                    \
	LONG  $0x0f3a0f66; WORD $0x04c6 \ // PALIGNR XMM0,XMM6,0x4            /* XTMP0 = W[-7]                           */
	ROLL  $23, R14                  \ // y1 = a >> (22-13)
	XORL  e, R13                    \ // y0 = e ^ (e >> (25-11))
	MOVL  f, R15                    \ // y2 = f
	ROLL  $27, R13                  \ // y0 = (e >> (11-6)) ^ (e >> (25-6))
	XORL  a, R14                    \ // y1 = a ^ (a >> (22-13)
	XORL  g, R15                    \ // y2 = f^g
	LONG  $0xc4fe0f66               \ // PADDD XMM0,XMM4                  /* XTMP0 = W[-7] + W[-16]                  */
	XORL  e, R13                    \ // y0 = e ^ (e >> (11-6)) ^ (e >> (25-6) )
	ANDL  e, R15                    \ // y2 = (f^g)&e
	ROLL  $21, R14                  \ // y1 = (a >> (13-2)) ^ (a >> (22-2))
	                                \
	\ // compute s0
	                                \
	MOVOU X5, X1                    \
	LONG  $0x0f3a0f66; WORD $0x04cc \ // PALIGNR XMM1,XMM4,0x4            /* XTMP1 = W[-15]                          */
	XORL  a, R14                    \ // y1 = a ^ (a >> (13-2)) ^ (a >> (22-2))
	ROLL  $26, R13                  \ // y0 = S1 = (e>>6) & (e>>11) ^ (e>>25)
	XORL  g, R15                    \ // y2 = CH = ((f^g)&e)^g
	ROLL  $30, R14                  \ // y1 = S0 = (a>>2) ^ (a>>13) ^ (a>>22)
	ADDL  R13, R15                  \ // y2 = S1 + CH
	ADDL  _xfer+48(FP), R15         \ // y2 = k + w + S1 + CH
	MOVL  a, R13                    \ // y0 = a
	ADDL  R15, h                    \ // h = h + S1 + CH + k + w
	\ // ROTATE_ARGS
	MOVL  a, R15                    \ // y2 = a
	MOVOU X1, X2                    \
	LONG  $0xd2720f66; BYTE $0x07   \ // PSRLD XMM2,0x7                   /*                                         */
	ORL   c, R13                    \ // y0 = a|c
	ADDL  h, d                      \ // d = d + h + S1 + CH + k + w
	ANDL  c, R15                    \ // y2 = a&c
	MOVOU X1, X3                    \
	LONG  $0xf3720f66; BYTE $0x19   \ // PSLLD XMM3,0x19                  /*                                         */
	ANDL  b, R13                    \ // y0 = (a|c)&b
	ADDL  R14, h                    \ // h = h + S1 + CH + k + w + S0
	LONG  $0xdaeb0f66               \ // POR   XMM3,XMM2                  /* XTMP1 = W[-15] MY_ROR 7                 */
	ORL   R15, R13                  \ // y0 = MAJ = (a|c)&b)|(a&c)
	ADDL  R13, h                    \ // h = h + S1 + CH + k + w + S0 + MAJ
	\ // ROTATE_ARGS
	MOVL  d, R13                    \ // y0 = e
	MOVL  h, R14                    \ // y1 = a
	ROLL  $18, R13                  \ // y0 = e >> (25-11)
	XORL  d, R13                    \ // y0 = e ^ (e >> (25-11))
	MOVL  e, R15                    \ // y2 = f
	ROLL  $23, R14                  \ // y1 = a >> (22-13)
	MOVOU X1, X2                    \
	LONG  $0xd2720f66; BYTE $0x12   \ // PSRLD XMM2,0x12                  /*                                         */
	XORL  h, R14                    \ // y1 = a ^ (a >> (22-13)
	ROLL  $27, R13                  \ // y0 = (e >> (11-6)) ^ (e >> (25-6))
	XORL  f, R15                    \ // y2 = f^g
	MOVOU X1, X8                    \
	LONG  $0x720f4166; WORD $0x03d0 \ // PSRLD XMM8,0x3                   /* XTMP4 = W[-15] >> 3                     */
	ROLL  $21, R14                  \ // y1 = (a >> (13-2)) ^ (a >> (22-2))
	XORL  d, R13                    \ // y0 = e ^ (e >> (11-6)) ^ (e >> (25-6))
	ANDL  d, R15                    \ // y2 = (f^g)&e
	ROLL  $26, R13                  \ // y0 = S1 = (e>>6) & (e>>11) ^ (e>>25)
	LONG  $0xf1720f66; BYTE $0x0e   \ // PSLLD XMM1,0xe                   /*                                         */
	XORL  h, R14                    \ // y1 = a ^ (a >> (13-2)) ^ (a >> (22-2))
	XORL  f, R15                    \ // y2 = CH = ((f^g)&e)^g
	LONG  $0xd9ef0f66               \ // PXOR  XMM3,XMM1                  /*                                         */
	ADDL  R13, R15                  \ // y2 = S1 + CH
	ADDL  _xfer+52(FP), R15         \ // y2 = k + w + S1 + CH
	ROLL  $30, R14                  \ // y1 = S0 = (a>>2) ^ (a>>13) ^ (a>>22)
	LONG  $0xdaef0f66               \ // PXOR  XMM3,XMM2                  /* XTMP1 = W[-15] MY_ROR 7 ^ W[-15] MY_ROR */
	MOVL  h, R13                    \ // y0 = a
	ADDL  R15, g                    \ // h = h + S1 + CH + k + w
	MOVL  h, R15                    \ // y2 = a
	MOVOU X3, X1                    \
	LONG  $0xef0f4166; BYTE $0xc8   \ // PXOR  XMM1,XMM8                  /* XTMP1 = s0                              */
	ORL   b, R13                    \ // y0 = a|c
	ADDL  g, c                      \ // d = d + h + S1 + CH + k + w
	ANDL  b, R15                    \ // y2 = a&c
	                                \
	\ // compute low s1
	                                \
	LONG  $0xd7700f66; BYTE $0xfa   \ // PSHUFD XMM2,XMM7,0xfa            /* XTMP2 = W[-2] {BBAA}                    */
	ANDL  a, R13                    \ // y0 = (a|c)&b
	ADDL  R14, g                    \ // h = h + S1 + CH + k + w + S0
	LONG  $0xc1fe0f66               \ // PADDD XMM0,XMM1                  /* XTMP0 = W[-16] + W[-7] + s0             */
	ORL   R15, R13                  \ // y0 = MAJ = (a|c)&b)|(a&c)
	ADDL  R13, g                    \ // h = h + S1 + CH + k + w + S0 + MAJ
	\ // ROTATE_ARGS
	MOVL  c, R13                    \ // y0 = e
	MOVL  g, R14                    \ // y1 = a
	ROLL  $18, R13                  \ // y0 = e >> (25-11)
	XORL  c, R13                    \ // y0 = e ^ (e >> (25-11))
	ROLL  $23, R14                  \ // y1 = a >> (22-13)
	MOVL  d, R15                    \ // y2 = f
	XORL  g, R14                    \ // y1 = a ^ (a >> (22-13)
	ROLL  $27, R13                  \ // y0 = (e >> (11-6)) ^ (e >> (25-6))
	MOVOU X2, X8                    \
	LONG  $0x720f4166; WORD $0x0ad0 \ // PSRLD XMM8,0xa                   /* XTMP4 = W[-2] >> 10 {BBAA}              */
	XORL  e, R15                    \ // y2 = f^g
	MOVOU X2, X3                    \
	LONG  $0xd3730f66; BYTE $0x13   \ // PSRLQ XMM3,0x13                  /* XTMP3 = W[-2] MY_ROR 19 {xBxA}          */
	XORL  c, R13                    \ // y0 = e ^ (e >> (11-6)) ^ (e >> (25-6))
	ANDL  c, R15                    \ // y2 = (f^g)&e
	LONG  $0xd2730f66; BYTE $0x11   \ // PSRLQ XMM2,0x11                  /* XTMP2 = W[-2] MY_ROR 17 {xBxA}          */
	ROLL  $21, R14                  \ // y1 = (a >> (13-2)) ^ (a >> (22-2))
	XORL  g, R14                    \ // y1 = a ^ (a >> (13-2)) ^ (a >> (22-2))
	XORL  e, R15                    \ // y2 = CH = ((f^g)&e)^g
	ROLL  $26, R13                  \ // y0 = S1 = (e>>6) & (e>>11) ^ (e>>25)
	LONG  $0xd3ef0f66               \ // PXOR  XMM2,XMM3                  /*                                         */
	ADDL  R13, R15                  \ // y2 = S1 + CH
	ROLL  $30, R14                  \ // y1 = S0 = (a>>2) ^ (a>>13) ^ (a>>22)
	ADDL  _xfer+56(FP), R15         \ // y2 = k + w + S1 + CH
	LONG  $0xef0f4466; BYTE $0xc2   \ // PXOR  XMM8,XMM2                  /* XTMP4 = s1 {xBxA}                       */
	MOVL  g, R13                    \ // y0 = a
	ADDL  R15, f                    \ // h = h + S1 + CH + k + w
	MOVL  g, R15                    \ // y2 = a
	LONG  $0x380f4566; WORD $0xc200 \ // PSHUFB XMM8,XMM10                /* XTMP4 = s1 {00BA}                       */
	ORL   a, R13                    \ // y0 = a|c
	ADDL  f, b                      \ // d = d + h + S1 + CH + k + w
	ANDL  a, R15                    \ // y2 = a&c
	LONG  $0xfe0f4166; BYTE $0xc0   \ // PADDD XMM0,XMM8                  /* XTMP0 = {..., ..., W[1], W[0]}          */
	ANDL  h, R13                    \ // y0 = (a|c)&b
	ADDL  R14, f                    \ // h = h + S1 + CH + k + w + S0
	                                \
	\ // compute high s1
	                                \
	LONG  $0xd0700f66; BYTE $0x50   \ // PSHUFD XMM2,XMM0,0x50            /* XTMP2 = W[-2] {DDCC}                    */
	ORL   R15, R13                  \ // y0 = MAJ = (a|c)&b)|(a&c)
	ADDL  R13, f                    \ // h = h + S1 + CH + k + w + S0 + MAJ
	\ // ROTATE_ARGS
	MOVL  b, R13                    \ // y0 = e
	ROLL  $18, R13                  \ // y0 = e >> (25-11)
	MOVL  f, R14                    \ // y1 = a
	ROLL  $23, R14                  \ // y1 = a >> (22-13)
	XORL  b, R13                    \ // y0 = e ^ (e >> (25-11))
	MOVL  c, R15                    \ // y2 = f
	ROLL  $27, R13                  \ // y0 = (e >> (11-6)) ^ (e >> (25-6))
	MOVOU X2, X11                   \
	LONG  $0x720f4166; WORD $0x0ad3 \ // PSRLD XMM11,0xa                  /* XTMP5 = W[-2] >> 10 {DDCC}              */
	XORL  f, R14                    \ // y1 = a ^ (a >> (22-13)
	XORL  d, R15                    \ // y2 = f^g
	MOVOU X2, X3                    \
	LONG  $0xd3730f66; BYTE $0x13   \ // PSRLQ XMM3,0x13                  /* XTMP3 = W[-2] MY_ROR 19 {xDxC}          */
	XORL  b, R13                    \ // y0 = e ^ (e >> (11-6)) ^ (e >> (25-6))
	ANDL  b, R15                    \ // y2 = (f^g)&e
	ROLL  $21, R14                  \ // y1 = (a >> (13-2)) ^ (a >> (22-2))
	LONG  $0xd2730f66; BYTE $0x11   \ // PSRLQ XMM2,0x11                  /* XTMP2 = W[-2] MY_ROR 17 {xDxC}          */
	XORL  f, R14                    \ // y1 = a ^ (a >> (13-2)) ^ (a >> (22-2))
	ROLL  $26, R13                  \ // y0 = S1 = (e>>6) & (e>>11) ^ (e>>25)
	XORL  d, R15                    \ // y2 = CH = ((f^g)&e)^g
	LONG  $0xd3ef0f66               \ // PXOR  XMM2,XMM3                  /*                                         */
	ROLL  $30, R14                  \ // y1 = S0 = (a>>2) ^ (a>>13) ^ (a>>22)
	ADDL  R13, R15                  \ // y2 = S1 + CH
	ADDL  _xfer+60(FP), R15         \ // y2 = k + w + S1 + CH
	LONG  $0xef0f4466; BYTE $0xda   \ // PXOR  XMM11,XMM2                 /* XTMP5 = s1 {xDxC}                       */
	MOVL  f, R13                    \ // y0 = a
	ADDL  R15, e                    \ // h = h + S1 + CH + k + w
	MOVL  f, R15                    \ // y2 = a
	LONG  $0x380f4566; WORD $0xdc00 \ // PSHUFB XMM11,XMM12               /* XTMP5 = s1 {DC00}                       */
	ORL   h, R13                    \ // y0 = a|c
	ADDL  e, a                      \ // d = d + h + S1 + CH + k + w
	ANDL  h, R15                    \ // y2 = a&c
	MOVOU X11, X4                   \
	LONG  $0xe0fe0f66               \ // PADDD XMM4,XMM0                  /* X0 = {W[3], W[2], W[1], W[0]}           */
	ANDL  g, R13                    \ // y0 = (a|c)&b
	ADDL  R14, e                    \ // h = h + S1 + CH + k + w + S0
	ORL   R15, R13                  \ // y0 = MAJ = (a|c)&b)|(a&c)
	ADDL  R13, e                    \ // h = h + S1 + CH + k + w + S0 + MAJ
	\ // ROTATE_ARGS
	ROTATE_XS

#define DO_ROUND(a, b, c, d, e, f, g, h, offset) \
	MOVL e, R13                \ // y0 = e
	ROLL $18, R13              \ // y0 = e >> (25-11)
	MOVL a, R14                \ // y1 = a
	XORL e, R13                \ // y0 = e ^ (e >> (25-11))
	ROLL $23, R14              \ // y1 = a >> (22-13)
	MOVL f, R15                \ // y2 = f
	XORL a, R14                \ // y1 = a ^ (a >> (22-13)
	ROLL $27, R13              \ // y0 = (e >> (11-6)) ^ (e >> (25-6))
	XORL g, R15                \ // y2 = f^g
	XORL e, R13                \ // y0 = e ^ (e >> (11-6)) ^ (e >> (25-6))
	ROLL $21, R14              \ // y1 = (a >> (13-2)) ^ (a >> (22-2))
	ANDL e, R15                \ // y2 = (f^g)&e
	XORL a, R14                \ // y1 = a ^ (a >> (13-2)) ^ (a >> (22-2))
	ROLL $26, R13              \ // y0 = S1 = (e>>6) & (e>>11) ^ (e>>25)
	XORL g, R15                \ // y2 = CH = ((f^g)&e)^g
	ADDL R13, R15              \ // y2 = S1 + CH
	ROLL $30, R14              \ // y1 = S0 = (a>>2) ^ (a>>13) ^ (a>>22)
	ADDL _xfer+offset(FP), R15 \ // y2 = k + w + S1 + CH
	MOVL a, R13                \ // y0 = a
	ADDL R15, h                \ // h = h + S1 + CH + k + w
	MOVL a, R15                \ // y2 = a
	ORL  c, R13                \ // y0 = a|c
	ADDL h, d                  \ // d = d + h + S1 + CH + k + w
	ANDL c, R15                \ // y2 = a&c
	ANDL b, R13                \ // y0 = (a|c)&b
	ADDL R14, h                \ // h = h + S1 + CH + k + w + S0
	ORL  R15, R13              \ // y0 = MAJ = (a|c)&b)|(a&c)
	ADDL R13, h                // h = h + S1 + CH + k + w + S0 + MAJ

// func blockSsse(h []uint32, message []uint8, reserved0, reserved1, reserved2, reserved3 uint64)
TEXT ·blockSsse(SB), 7, $0-80

	MOVQ h+0(FP), SI             // SI: &h
	MOVQ message_base+24(FP), R8 // &message
	MOVQ message_len+32(FP), R9  // length of message
	CMPQ R9, $0
	JEQ  done_hash
	ADDQ R8, R9
	MOVQ R9, reserved2+64(FP)    // store end of message

	// Register definition
	//  a -->  eax
	//  b -->  ebx
	//  c -->  ecx
	//  d -->  r8d
	//  e -->  edx
	//  f -->  r9d
	//  g --> r10d
	//  h --> r11d
	//
	// y0 --> r13d
	// y1 --> r14d
	// y2 --> r15d

	MOVL (0*4)(SI), AX  // a = H0
	MOVL (1*4)(SI), BX  // b = H1
	MOVL (2*4)(SI), CX  // c = H2
	MOVL (3*4)(SI), R8  // d = H3
	MOVL (4*4)(SI), DX  // e = H4
	MOVL (5*4)(SI), R9  // f = H5
	MOVL (6*4)(SI), R10 // g = H6
	MOVL (7*4)(SI), R11 // h = H7

	MOVOU bflipMask<>(SB), X13
	MOVOU shuf00BA<>(SB), X10  // shuffle xBxA -> 00BA
	MOVOU shufDC00<>(SB), X12  // shuffle xDxC -> DC00

	MOVQ message_base+24(FP), SI // SI: &message

loop0:
	LEAQ constants<>(SB), BP

	// byte swap first 16 dwords
	MOVOU 0*16(SI), X4
	LONG  $0x380f4166; WORD $0xe500 // PSHUFB XMM4, XMM13
	MOVOU 1*16(SI), X5
	LONG  $0x380f4166; WORD $0xed00 // PSHUFB XMM5, XMM13
	MOVOU 2*16(SI), X6
	LONG  $0x380f4166; WORD $0xf500 // PSHUFB XMM6, XMM13
	MOVOU 3*16(SI), X7
	LONG  $0x380f4166; WORD $0xfd00 // PSHUFB XMM7, XMM13

	MOVQ SI, reserved3+72(FP)
	MOVD $0x3, DI

	// Align
	//  nop    WORD PTR [rax+rax*1+0x0]

	// schedule 48 input dwords, by doing 3 rounds of 16 each
loop1:
	MOVOU X4, X9
	LONG  $0xfe0f4466; WORD $0x004d // PADDD XMM9, 0[RBP]   /* Add 1st constant to first part of message */
	MOVOU X9, reserved0+48(FP)
	FOUR_ROUNDS_AND_SCHED(AX, BX,  CX,  R8, DX, R9, R10, R11)

	MOVOU X4, X9
	LONG  $0xfe0f4466; WORD $0x104d // PADDD XMM9, 16[RBP]   /* Add 2nd constant to message */
	MOVOU X9, reserved0+48(FP)
	FOUR_ROUNDS_AND_SCHED(DX, R9, R10, R11, AX, BX,  CX,  R8)

	MOVOU X4, X9
	LONG  $0xfe0f4466; WORD $0x204d // PADDD XMM9, 32[RBP]   /* Add 3rd constant to message */
	MOVOU X9, reserved0+48(FP)
	FOUR_ROUNDS_AND_SCHED(AX, BX,  CX,  R8, DX, R9, R10, R11)

	MOVOU X4, X9
	LONG  $0xfe0f4466; WORD $0x304d // PADDD XMM9, 48[RBP]   /* Add 4th constant to message */
	MOVOU X9, reserved0+48(FP)
	ADDQ  $64, BP
	FOUR_ROUNDS_AND_SCHED(DX, R9, R10, R11, AX, BX,  CX,  R8)

	SUBQ $1, DI
	JNE  loop1

	MOVD $0x2, DI

loop2:
	MOVOU X4, X9
	LONG  $0xfe0f4466; WORD $0x004d // PADDD XMM9, 0[RBP]   /* Add 1st constant to first part of message */
	MOVOU X9, reserved0+48(FP)
	DO_ROUND( AX,  BX,  CX,  R8,  DX,  R9, R10, R11, 48)
	DO_ROUND(R11,  AX,  BX,  CX,  R8,  DX,  R9, R10, 52)
	DO_ROUND(R10, R11,  AX,  BX,  CX,  R8,  DX,  R9, 56)
	DO_ROUND( R9, R10, R11,  AX,  BX,  CX,  R8,  DX, 60)

	MOVOU X5, X9
	LONG  $0xfe0f4466; WORD $0x104d // PADDD XMM9, 16[RBP]   /* Add 2nd constant to message */
	MOVOU X9, reserved0+48(FP)
	ADDQ  $32, BP
	DO_ROUND( DX,  R9, R10, R11,  AX,  BX,  CX,  R8, 48)
	DO_ROUND( R8,  DX,  R9, R10, R11,  AX,  BX,  CX, 52)
	DO_ROUND( CX,  R8,  DX,  R9, R10, R11,  AX,  BX, 56)
	DO_ROUND( BX,  CX,  R8,  DX,  R9, R10, R11,  AX, 60)

	MOVOU X6, X4
	MOVOU X7, X5

	SUBQ $1, DI
	JNE  loop2

	MOVQ h+0(FP), SI    // SI: &h
	ADDL (0*4)(SI), AX  // H0 = a + H0
	MOVL AX, (0*4)(SI)
	ADDL (1*4)(SI), BX  // H1 = b + H1
	MOVL BX, (1*4)(SI)
	ADDL (2*4)(SI), CX  // H2 = c + H2
	MOVL CX, (2*4)(SI)
	ADDL (3*4)(SI), R8  // H3 = d + H3
	MOVL R8, (3*4)(SI)
	ADDL (4*4)(SI), DX  // H4 = e + H4
	MOVL DX, (4*4)(SI)
	ADDL (5*4)(SI), R9  // H5 = f + H5
	MOVL R9, (5*4)(SI)
	ADDL (6*4)(SI), R10 // H6 = g + H6
	MOVL R10, (6*4)(SI)
	ADDL (7*4)(SI), R11 // H7 = h + H7
	MOVL R11, (7*4)(SI)

	MOVQ reserved3+72(FP), SI
	ADDQ $64, SI
	CMPQ reserved2+64(FP), SI
	JNE  loop0

done_hash:
	RET

// Constants table
DATA constants<>+0x0(SB)/8, $0x71374491428a2f98
DATA constants<>+0x8(SB)/8, $0xe9b5dba5b5c0fbcf
DATA constants<>+0x10(SB)/8, $0x59f111f13956c25b
DATA constants<>+0x18(SB)/8, $0xab1c5ed5923f82a4
DATA constants<>+0x20(SB)/8, $0x12835b01d807aa98
DATA constants<>+0x28(SB)/8, $0x550c7dc3243185be
DATA constants<>+0x30(SB)/8, $0x80deb1fe72be5d74
DATA constants<>+0x38(SB)/8, $0xc19bf1749bdc06a7
DATA constants<>+0x40(SB)/8, $0xefbe4786e49b69c1
DATA constants<>+0x48(SB)/8, $0x240ca1cc0fc19dc6
DATA constants<>+0x50(SB)/8, $0x4a7484aa2de92c6f
DATA constants<>+0x58(SB)/8, $0x76f988da5cb0a9dc
DATA constants<>+0x60(SB)/8, $0xa831c66d983e5152
DATA constants<>+0x68(SB)/8, $0xbf597fc7b00327c8
DATA constants<>+0x70(SB)/8, $0xd5a79147c6e00bf3
DATA constants<>+0x78(SB)/8, $0x1429296706ca6351
DATA constants<>+0x80(SB)/8, $0x2e1b213827b70a85
DATA constants<>+0x88(SB)/8, $0x53380d134d2c6dfc
DATA constants<>+0x90(SB)/8, $0x766a0abb650a7354
DATA constants<>+0x98(SB)/8, $0x92722c8581c2c92e
DATA constants<>+0xa0(SB)/8, $0xa81a664ba2bfe8a1
DATA constants<>+0xa8(SB)/8, $0xc76c51a3c24b8b70
DATA constants<>+0xb0(SB)/8, $0xd6990624d192e819
DATA constants<>+0xb8(SB)/8, $0x106aa070f40e3585
DATA constants<>+0xc0(SB)/8, $0x1e376c0819a4c116
DATA constants<>+0xc8(SB)/8, $0x34b0bcb52748774c
DATA constants<>+0xd0(SB)/8, $0x4ed8aa4a391c0cb3
DATA constants<>+0xd8(SB)/8, $0x682e6ff35b9cca4f
DATA constants<>+0xe0(SB)/8, $0x78a5636f748f82ee
DATA constants<>+0xe8(SB)/8, $0x8cc7020884c87814
DATA constants<>+0xf0(SB)/8, $0xa4506ceb90befffa
DATA constants<>+0xf8(SB)/8, $0xc67178f2bef9a3f7

DATA bflipMask<>+0x00(SB)/8, $0x0405060700010203
DATA bflipMask<>+0x08(SB)/8, $0x0c0d0e0f08090a0b

DATA shuf00BA<>+0x00(SB)/8, $0x0b0a090803020100
DATA shuf00BA<>+0x08(SB)/8, $0xFFFFFFFFFFFFFFFF

DATA shufDC00<>+0x00(SB)/8, $0xFFFFFFFFFFFFFFFF
DATA shufDC00<>+0x08(SB)/8, $0x0b0a090803020100

GLOBL constants<>(SB), 8, $256
GLOBL bflipMask<>(SB), (NOPTR+RODATA), $16
GLOBL shuf00BA<>(SB), (NOPTR+RODATA), $16
GLOBL shufDC00<>(SB), (NOPTR+RODATA), $16
