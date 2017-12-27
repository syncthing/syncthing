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

package sha256

// True when SIMD instructions are available.
var avx512 = haveAVX512()
var avx2 = haveAVX2()
var avx = haveAVX()
var ssse3 = haveSSSE3()
var armSha = haveArmSha()

// haveAVX returns true when there is AVX support
func haveAVX() bool {
	_, _, c, _ := cpuid(1)

	// Check XGETBV, OXSAVE and AVX bits
	if c&(1<<26) != 0 && c&(1<<27) != 0 && c&(1<<28) != 0 {
		// Check for OS support
		eax, _ := xgetbv(0)
		return (eax & 0x6) == 0x6
	}
	return false
}

// haveAVX2 returns true when there is AVX2 support
func haveAVX2() bool {
	mfi, _, _, _ := cpuid(0)

	// Check AVX2, AVX2 requires OS support, but BMI1/2 don't.
	if mfi >= 7 && haveAVX() {
		_, ebx, _, _ := cpuidex(7, 0)
		return (ebx & 0x00000020) != 0
	}
	return false
}

// haveAVX512 returns true when there is AVX512 support
func haveAVX512() bool {
	mfi, _, _, _ := cpuid(0)

	// Check AVX2, AVX2 requires OS support, but BMI1/2 don't.
	if mfi >= 7 {
		_, _, c, _ := cpuid(1)

		// Only detect AVX-512 features if XGETBV is supported
		if c&((1<<26)|(1<<27)) == (1<<26)|(1<<27) {
			// Check for OS support
			eax, _ := xgetbv(0)
			_, ebx, _, _ := cpuidex(7, 0)

			// Verify that XCR0[7:5] = ‘111b’ (OPMASK state, upper 256-bit of ZMM0-ZMM15 and
			// ZMM16-ZMM31 state are enabled by OS)
			/// and that XCR0[2:1] = ‘11b’ (XMM state and YMM state are enabled by OS).
			if (eax>>5)&7 == 7 && (eax>>1)&3 == 3 {
				if ebx&(1<<16) == 0 {
					return false // no AVX512F
				}
				if ebx&(1<<17) == 0 {
					return false // no AVX512DQ
				}
				if ebx&(1<<30) == 0 {
					return false // no AVX512BW
				}
				if ebx&(1<<31) == 0 {
					return false // no AVX512VL
				}
				return true
			}
		}
	}
	return false
}

// haveSSSE3 returns true when there is SSSE3 support
func haveSSSE3() bool {

	_, _, c, _ := cpuid(1)

	return (c & 0x00000200) != 0
}
