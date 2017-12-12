//+build !noasm !appengine

// ARM64 version of SHA256

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
// Based on implementation as found in https://github.com/jocover/sha256-armv8
//
// Use github.com/minio/asm2plan9s on this file to assemble ARM instructions to
// their Plan9 equivalents
//

TEXT ·blockArm(SB), 7, $0
	MOVD h+0(FP), R0
	MOVD message+24(FP), R1
	MOVD lenmessage+32(FP), R2 // length of message
	SUBS $64, R2
	BMI  complete

	// Load constants table pointer
	MOVD $·constants(SB), R3

	// Cache constants table in registers v16 - v31
	WORD $0x4cdf2870 // ld1	{v16.4s-v19.4s}, [x3], #64
	WORD $0x4cdf7800 // ld1	{v0.4s}, [x0], #16
	WORD $0x4cdf2874 // ld1	{v20.4s-v23.4s}, [x3], #64

	WORD $0x4c407801 // ld1	{v1.4s}, [x0]
	WORD $0x4cdf2878 // ld1	{v24.4s-v27.4s}, [x3], #64
	WORD $0xd1004000 // sub	x0, x0, #0x10
	WORD $0x4cdf287c // ld1	{v28.4s-v31.4s}, [x3], #64

loop:
	// Main loop
	WORD $0x4cdf2025 // ld1	{v5.16b-v8.16b}, [x1], #64
	WORD $0x4ea01c02 // mov	v2.16b, v0.16b
	WORD $0x4ea11c23 // mov	v3.16b, v1.16b
	WORD $0x6e2008a5 // rev32	v5.16b, v5.16b
	WORD $0x6e2008c6 // rev32	v6.16b, v6.16b
	WORD $0x4eb084a9 // add	v9.4s, v5.4s, v16.4s
	WORD $0x6e2008e7 // rev32	v7.16b, v7.16b
	WORD $0x4eb184ca // add	v10.4s, v6.4s, v17.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e094062 // sha256h	q2, q3, v9.4s
	WORD $0x5e095083 // sha256h2	q3, q4, v9.4s
	WORD $0x5e2828c5 // sha256su0	v5.4s, v6.4s
	WORD $0x6e200908 // rev32	v8.16b, v8.16b
	WORD $0x4eb284e9 // add	v9.4s, v7.4s, v18.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e0a4062 // sha256h	q2, q3, v10.4s
	WORD $0x5e0a5083 // sha256h2	q3, q4, v10.4s
	WORD $0x5e2828e6 // sha256su0	v6.4s, v7.4s
	WORD $0x5e0860e5 // sha256su1	v5.4s, v7.4s, v8.4s
	WORD $0x4eb3850a // add	v10.4s, v8.4s, v19.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e094062 // sha256h	q2, q3, v9.4s
	WORD $0x5e095083 // sha256h2	q3, q4, v9.4s
	WORD $0x5e282907 // sha256su0	v7.4s, v8.4s
	WORD $0x5e056106 // sha256su1	v6.4s, v8.4s, v5.4s
	WORD $0x4eb484a9 // add	v9.4s, v5.4s, v20.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e0a4062 // sha256h	q2, q3, v10.4s
	WORD $0x5e0a5083 // sha256h2	q3, q4, v10.4s
	WORD $0x5e2828a8 // sha256su0	v8.4s, v5.4s
	WORD $0x5e0660a7 // sha256su1	v7.4s, v5.4s, v6.4s
	WORD $0x4eb584ca // add	v10.4s, v6.4s, v21.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e094062 // sha256h	q2, q3, v9.4s
	WORD $0x5e095083 // sha256h2	q3, q4, v9.4s
	WORD $0x5e2828c5 // sha256su0	v5.4s, v6.4s
	WORD $0x5e0760c8 // sha256su1	v8.4s, v6.4s, v7.4s
	WORD $0x4eb684e9 // add	v9.4s, v7.4s, v22.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e0a4062 // sha256h	q2, q3, v10.4s
	WORD $0x5e0a5083 // sha256h2	q3, q4, v10.4s
	WORD $0x5e2828e6 // sha256su0	v6.4s, v7.4s
	WORD $0x5e0860e5 // sha256su1	v5.4s, v7.4s, v8.4s
	WORD $0x4eb7850a // add	v10.4s, v8.4s, v23.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e094062 // sha256h	q2, q3, v9.4s
	WORD $0x5e095083 // sha256h2	q3, q4, v9.4s
	WORD $0x5e282907 // sha256su0	v7.4s, v8.4s
	WORD $0x5e056106 // sha256su1	v6.4s, v8.4s, v5.4s
	WORD $0x4eb884a9 // add	v9.4s, v5.4s, v24.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e0a4062 // sha256h	q2, q3, v10.4s
	WORD $0x5e0a5083 // sha256h2	q3, q4, v10.4s
	WORD $0x5e2828a8 // sha256su0	v8.4s, v5.4s
	WORD $0x5e0660a7 // sha256su1	v7.4s, v5.4s, v6.4s
	WORD $0x4eb984ca // add	v10.4s, v6.4s, v25.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e094062 // sha256h	q2, q3, v9.4s
	WORD $0x5e095083 // sha256h2	q3, q4, v9.4s
	WORD $0x5e2828c5 // sha256su0	v5.4s, v6.4s
	WORD $0x5e0760c8 // sha256su1	v8.4s, v6.4s, v7.4s
	WORD $0x4eba84e9 // add	v9.4s, v7.4s, v26.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e0a4062 // sha256h	q2, q3, v10.4s
	WORD $0x5e0a5083 // sha256h2	q3, q4, v10.4s
	WORD $0x5e2828e6 // sha256su0	v6.4s, v7.4s
	WORD $0x5e0860e5 // sha256su1	v5.4s, v7.4s, v8.4s
	WORD $0x4ebb850a // add	v10.4s, v8.4s, v27.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e094062 // sha256h	q2, q3, v9.4s
	WORD $0x5e095083 // sha256h2	q3, q4, v9.4s
	WORD $0x5e282907 // sha256su0	v7.4s, v8.4s
	WORD $0x5e056106 // sha256su1	v6.4s, v8.4s, v5.4s
	WORD $0x4ebc84a9 // add	v9.4s, v5.4s, v28.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e0a4062 // sha256h	q2, q3, v10.4s
	WORD $0x5e0a5083 // sha256h2	q3, q4, v10.4s
	WORD $0x5e2828a8 // sha256su0	v8.4s, v5.4s
	WORD $0x5e0660a7 // sha256su1	v7.4s, v5.4s, v6.4s
	WORD $0x4ebd84ca // add	v10.4s, v6.4s, v29.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e094062 // sha256h	q2, q3, v9.4s
	WORD $0x5e095083 // sha256h2	q3, q4, v9.4s
	WORD $0x5e0760c8 // sha256su1	v8.4s, v6.4s, v7.4s
	WORD $0x4ebe84e9 // add	v9.4s, v7.4s, v30.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e0a4062 // sha256h	q2, q3, v10.4s
	WORD $0x5e0a5083 // sha256h2	q3, q4, v10.4s
	WORD $0x4ebf850a // add	v10.4s, v8.4s, v31.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e094062 // sha256h	q2, q3, v9.4s
	WORD $0x5e095083 // sha256h2	q3, q4, v9.4s
	WORD $0x4ea21c44 // mov	v4.16b, v2.16b
	WORD $0x5e0a4062 // sha256h	q2, q3, v10.4s
	WORD $0x5e0a5083 // sha256h2	q3, q4, v10.4s
	WORD $0x4ea38421 // add	v1.4s, v1.4s, v3.4s
	WORD $0x4ea28400 // add	v0.4s, v0.4s, v2.4s

	SUBS $64, R2
	BPL  loop

	// Store result
	WORD $0x4c00a800 // st1	{v0.4s, v1.4s}, [x0]

complete:
	RET


// Constants table
DATA ·constants+0x0(SB)/8, $0x71374491428a2f98
DATA ·constants+0x8(SB)/8, $0xe9b5dba5b5c0fbcf
DATA ·constants+0x10(SB)/8, $0x59f111f13956c25b
DATA ·constants+0x18(SB)/8, $0xab1c5ed5923f82a4
DATA ·constants+0x20(SB)/8, $0x12835b01d807aa98
DATA ·constants+0x28(SB)/8, $0x550c7dc3243185be
DATA ·constants+0x30(SB)/8, $0x80deb1fe72be5d74
DATA ·constants+0x38(SB)/8, $0xc19bf1749bdc06a7
DATA ·constants+0x40(SB)/8, $0xefbe4786e49b69c1
DATA ·constants+0x48(SB)/8, $0x240ca1cc0fc19dc6
DATA ·constants+0x50(SB)/8, $0x4a7484aa2de92c6f
DATA ·constants+0x58(SB)/8, $0x76f988da5cb0a9dc
DATA ·constants+0x60(SB)/8, $0xa831c66d983e5152
DATA ·constants+0x68(SB)/8, $0xbf597fc7b00327c8
DATA ·constants+0x70(SB)/8, $0xd5a79147c6e00bf3
DATA ·constants+0x78(SB)/8, $0x1429296706ca6351
DATA ·constants+0x80(SB)/8, $0x2e1b213827b70a85
DATA ·constants+0x88(SB)/8, $0x53380d134d2c6dfc
DATA ·constants+0x90(SB)/8, $0x766a0abb650a7354
DATA ·constants+0x98(SB)/8, $0x92722c8581c2c92e
DATA ·constants+0xa0(SB)/8, $0xa81a664ba2bfe8a1
DATA ·constants+0xa8(SB)/8, $0xc76c51a3c24b8b70
DATA ·constants+0xb0(SB)/8, $0xd6990624d192e819
DATA ·constants+0xb8(SB)/8, $0x106aa070f40e3585
DATA ·constants+0xc0(SB)/8, $0x1e376c0819a4c116
DATA ·constants+0xc8(SB)/8, $0x34b0bcb52748774c
DATA ·constants+0xd0(SB)/8, $0x4ed8aa4a391c0cb3
DATA ·constants+0xd8(SB)/8, $0x682e6ff35b9cca4f
DATA ·constants+0xe0(SB)/8, $0x78a5636f748f82ee
DATA ·constants+0xe8(SB)/8, $0x8cc7020884c87814
DATA ·constants+0xf0(SB)/8, $0xa4506ceb90befffa
DATA ·constants+0xf8(SB)/8, $0xc67178f2bef9a3f7

GLOBL ·constants(SB), 8, $256

