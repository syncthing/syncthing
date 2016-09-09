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

// haveSSSE3 returns true when there is SSSE3 support
func haveSSSE3() bool {

	_, _, c, _ := cpuid(1)

	return (c & 0x00000200) != 0
}
