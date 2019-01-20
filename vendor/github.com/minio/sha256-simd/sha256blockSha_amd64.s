//+build !noasm !appengine

// SHA intrinsic version of SHA256

// Kristofer Peterson, (C) 2018.
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

#include "textflag.h"

DATA K<>+0x00(SB)/4, $0x428a2f98
DATA K<>+0x04(SB)/4, $0x71374491
DATA K<>+0x08(SB)/4, $0xb5c0fbcf
DATA K<>+0x0c(SB)/4, $0xe9b5dba5
DATA K<>+0x10(SB)/4, $0x3956c25b
DATA K<>+0x14(SB)/4, $0x59f111f1
DATA K<>+0x18(SB)/4, $0x923f82a4
DATA K<>+0x1c(SB)/4, $0xab1c5ed5
DATA K<>+0x20(SB)/4, $0xd807aa98
DATA K<>+0x24(SB)/4, $0x12835b01
DATA K<>+0x28(SB)/4, $0x243185be
DATA K<>+0x2c(SB)/4, $0x550c7dc3
DATA K<>+0x30(SB)/4, $0x72be5d74
DATA K<>+0x34(SB)/4, $0x80deb1fe
DATA K<>+0x38(SB)/4, $0x9bdc06a7
DATA K<>+0x3c(SB)/4, $0xc19bf174
DATA K<>+0x40(SB)/4, $0xe49b69c1
DATA K<>+0x44(SB)/4, $0xefbe4786
DATA K<>+0x48(SB)/4, $0x0fc19dc6
DATA K<>+0x4c(SB)/4, $0x240ca1cc
DATA K<>+0x50(SB)/4, $0x2de92c6f
DATA K<>+0x54(SB)/4, $0x4a7484aa
DATA K<>+0x58(SB)/4, $0x5cb0a9dc
DATA K<>+0x5c(SB)/4, $0x76f988da
DATA K<>+0x60(SB)/4, $0x983e5152
DATA K<>+0x64(SB)/4, $0xa831c66d
DATA K<>+0x68(SB)/4, $0xb00327c8
DATA K<>+0x6c(SB)/4, $0xbf597fc7
DATA K<>+0x70(SB)/4, $0xc6e00bf3
DATA K<>+0x74(SB)/4, $0xd5a79147
DATA K<>+0x78(SB)/4, $0x06ca6351
DATA K<>+0x7c(SB)/4, $0x14292967
DATA K<>+0x80(SB)/4, $0x27b70a85
DATA K<>+0x84(SB)/4, $0x2e1b2138
DATA K<>+0x88(SB)/4, $0x4d2c6dfc
DATA K<>+0x8c(SB)/4, $0x53380d13
DATA K<>+0x90(SB)/4, $0x650a7354
DATA K<>+0x94(SB)/4, $0x766a0abb
DATA K<>+0x98(SB)/4, $0x81c2c92e
DATA K<>+0x9c(SB)/4, $0x92722c85
DATA K<>+0xa0(SB)/4, $0xa2bfe8a1
DATA K<>+0xa4(SB)/4, $0xa81a664b
DATA K<>+0xa8(SB)/4, $0xc24b8b70
DATA K<>+0xac(SB)/4, $0xc76c51a3
DATA K<>+0xb0(SB)/4, $0xd192e819
DATA K<>+0xb4(SB)/4, $0xd6990624
DATA K<>+0xb8(SB)/4, $0xf40e3585
DATA K<>+0xbc(SB)/4, $0x106aa070
DATA K<>+0xc0(SB)/4, $0x19a4c116
DATA K<>+0xc4(SB)/4, $0x1e376c08
DATA K<>+0xc8(SB)/4, $0x2748774c
DATA K<>+0xcc(SB)/4, $0x34b0bcb5
DATA K<>+0xd0(SB)/4, $0x391c0cb3
DATA K<>+0xd4(SB)/4, $0x4ed8aa4a
DATA K<>+0xd8(SB)/4, $0x5b9cca4f
DATA K<>+0xdc(SB)/4, $0x682e6ff3
DATA K<>+0xe0(SB)/4, $0x748f82ee
DATA K<>+0xe4(SB)/4, $0x78a5636f
DATA K<>+0xe8(SB)/4, $0x84c87814
DATA K<>+0xec(SB)/4, $0x8cc70208
DATA K<>+0xf0(SB)/4, $0x90befffa
DATA K<>+0xf4(SB)/4, $0xa4506ceb
DATA K<>+0xf8(SB)/4, $0xbef9a3f7
DATA K<>+0xfc(SB)/4, $0xc67178f2
GLOBL K<>(SB), RODATA|NOPTR, $256

DATA SHUF_MASK<>+0x00(SB)/8, $0x0405060700010203
DATA SHUF_MASK<>+0x08(SB)/8, $0x0c0d0e0f08090a0b
GLOBL SHUF_MASK<>(SB), RODATA|NOPTR, $16

// Register Usage
// BX  base address of constant table (constant)
// DX  hash_state (constant)
// SI  hash_data.data
// DI  hash_data.data + hash_data.length - 64 (constant)
// X0  scratch
// X1  scratch
// X2  working hash state // ABEF
// X3  working hash state // CDGH
// X4  first 16 bytes of block
// X5  second 16 bytes of block
// X6  third 16 bytes of block
// X7  fourth 16 bytes of block
// X12 saved hash state // ABEF
// X13 saved hash state // CDGH
// X15 data shuffle mask (constant)

TEXT ·blockSha(SB), NOSPLIT, $0-32
	MOVQ      h+0(FP), DX
	MOVQ      message_base+8(FP), SI
	MOVQ      message_len+16(FP), DI
	LEAQ      -64(SI)(DI*1), DI
	MOVOU     (DX), X2
	MOVOU     16(DX), X1
	MOVO      X2, X3
	PUNPCKLLQ X1, X2
	PUNPCKHLQ X1, X3
	PSHUFD    $0x27, X2, X2
	PSHUFD    $0x27, X3, X3
	MOVO      SHUF_MASK<>(SB), X15
	LEAQ      K<>(SB), BX

	JMP TEST

LOOP:
	MOVO X2, X12
	MOVO X3, X13

	// load block and shuffle
	MOVOU  (SI), X4
	MOVOU  16(SI), X5
	MOVOU  32(SI), X6
	MOVOU  48(SI), X7
	PSHUFB X15, X4
	PSHUFB X15, X5
	PSHUFB X15, X6
	PSHUFB X15, X7

#define ROUND456 \
	PADDL  X5, X0                    \
	LONG   $0xdacb380f               \ // SHA256RNDS2 XMM3, XMM2
	MOVO   X5, X1                    \
	LONG   $0x0f3a0f66; WORD $0x04cc \ // PALIGNR XMM1, XMM4, 4
	PADDL  X1, X6                    \
	LONG   $0xf5cd380f               \ // SHA256MSG2 XMM6, XMM5
	PSHUFD $0x4e, X0, X0             \
	LONG   $0xd3cb380f               \ // SHA256RNDS2 XMM2, XMM3
	LONG   $0xe5cc380f               // SHA256MSG1 XMM4, XMM5

#define ROUND567 \
	PADDL  X6, X0                    \
	LONG   $0xdacb380f               \ // SHA256RNDS2 XMM3, XMM2
	MOVO   X6, X1                    \
	LONG   $0x0f3a0f66; WORD $0x04cd \ // PALIGNR XMM1, XMM5, 4
	PADDL  X1, X7                    \
	LONG   $0xfecd380f               \ // SHA256MSG2 XMM7, XMM6
	PSHUFD $0x4e, X0, X0             \
	LONG   $0xd3cb380f               \ // SHA256RNDS2 XMM2, XMM3
	LONG   $0xeecc380f               // SHA256MSG1 XMM5, XMM6

#define ROUND674 \
	PADDL  X7, X0                    \
	LONG   $0xdacb380f               \ // SHA256RNDS2 XMM3, XMM2
	MOVO   X7, X1                    \
	LONG   $0x0f3a0f66; WORD $0x04ce \ // PALIGNR XMM1, XMM6, 4
	PADDL  X1, X4                    \
	LONG   $0xe7cd380f               \ // SHA256MSG2 XMM4, XMM7
	PSHUFD $0x4e, X0, X0             \
	LONG   $0xd3cb380f               \ // SHA256RNDS2 XMM2, XMM3
	LONG   $0xf7cc380f               // SHA256MSG1 XMM6, XMM7

#define ROUND745 \
	PADDL  X4, X0                    \
	LONG   $0xdacb380f               \ // SHA256RNDS2 XMM3, XMM2
	MOVO   X4, X1                    \
	LONG   $0x0f3a0f66; WORD $0x04cf \ // PALIGNR XMM1, XMM7, 4
	PADDL  X1, X5                    \
	LONG   $0xeccd380f               \ // SHA256MSG2 XMM5, XMM4
	PSHUFD $0x4e, X0, X0             \
	LONG   $0xd3cb380f               \ // SHA256RNDS2 XMM2, XMM3
	LONG   $0xfccc380f               // SHA256MSG1 XMM7, XMM4

	// rounds 0-3
	MOVO   (BX), X0
	PADDL  X4, X0
	LONG   $0xdacb380f   // SHA256RNDS2 XMM3, XMM2
	PSHUFD $0x4e, X0, X0
	LONG   $0xd3cb380f   // SHA256RNDS2 XMM2, XMM3

	// rounds 4-7
	MOVO   1*16(BX), X0
	PADDL  X5, X0
	LONG   $0xdacb380f   // SHA256RNDS2 XMM3, XMM2
	PSHUFD $0x4e, X0, X0
	LONG   $0xd3cb380f   // SHA256RNDS2 XMM2, XMM3
	LONG   $0xe5cc380f   // SHA256MSG1 XMM4, XMM5

	// rounds 8-11
	MOVO   2*16(BX), X0
	PADDL  X6, X0
	LONG   $0xdacb380f   // SHA256RNDS2 XMM3, XMM2
	PSHUFD $0x4e, X0, X0
	LONG   $0xd3cb380f   // SHA256RNDS2 XMM2, XMM3
	LONG   $0xeecc380f   // SHA256MSG1 XMM5, XMM6

	MOVO 3*16(BX), X0; ROUND674  // rounds 12-15
	MOVO 4*16(BX), X0; ROUND745  // rounds 16-19
	MOVO 5*16(BX), X0; ROUND456  // rounds 20-23
	MOVO 6*16(BX), X0; ROUND567  // rounds 24-27
	MOVO 7*16(BX), X0; ROUND674  // rounds 28-31
	MOVO 8*16(BX), X0; ROUND745  // rounds 32-35
	MOVO 9*16(BX), X0; ROUND456  // rounds 36-39
	MOVO 10*16(BX), X0; ROUND567 // rounds 40-43
	MOVO 11*16(BX), X0; ROUND674 // rounds 44-47
	MOVO 12*16(BX), X0; ROUND745 // rounds 48-51

	// rounds 52-55
	MOVO   13*16(BX), X0
	PADDL  X5, X0
	LONG   $0xdacb380f               // SHA256RNDS2 XMM3, XMM2
	MOVO   X5, X1
	LONG   $0x0f3a0f66; WORD $0x04cc // PALIGNR XMM1, XMM4, 4
	PADDL  X1, X6
	LONG   $0xf5cd380f               // SHA256MSG2 XMM6, XMM5
	PSHUFD $0x4e, X0, X0
	LONG   $0xd3cb380f               // SHA256RNDS2 XMM2, XMM3

	// rounds 56-59
	MOVO   14*16(BX), X0
	PADDL  X6, X0
	LONG   $0xdacb380f               // SHA256RNDS2 XMM3, XMM2
	MOVO   X6, X1
	LONG   $0x0f3a0f66; WORD $0x04cd // PALIGNR XMM1, XMM5, 4
	PADDL  X1, X7
	LONG   $0xfecd380f               // SHA256MSG2 XMM7, XMM6
	PSHUFD $0x4e, X0, X0
	LONG   $0xd3cb380f               // SHA256RNDS2 XMM2, XMM3

	// rounds 60-63
	MOVO   15*16(BX), X0
	PADDL  X7, X0
	LONG   $0xdacb380f   // SHA256RNDS2 XMM3, XMM2
	PSHUFD $0x4e, X0, X0
	LONG   $0xd3cb380f   // SHA256RNDS2 XMM2, XMM3

	PADDL X12, X2
	PADDL X13, X3

	ADDQ $64, SI

TEST:
	CMPQ SI, DI
	JBE  LOOP

	PSHUFD $0x4e, X3, X0
	LONG   $0x0e3a0f66; WORD $0xf0c2 // PBLENDW XMM0, XMM2, 0xf0
	PSHUFD $0x4e, X2, X1
	LONG   $0x0e3a0f66; WORD $0x0fcb // PBLENDW XMM1, XMM3, 0x0f
	PSHUFD $0x1b, X0, X0
	PSHUFD $0x1b, X1, X1

	MOVOU X0, (DX)
	MOVOU X1, 16(DX)

	RET
