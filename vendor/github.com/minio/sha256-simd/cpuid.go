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
var avx512 bool
var avx2 bool
var avx bool
var sse bool
var sse2 bool
var sse3 bool
var ssse3 bool
var sse41 bool
var sse42 bool
var popcnt bool
var sha bool
var armSha = haveArmSha()

func init() {
	var _xsave bool
	var _osxsave bool
	var _avx bool
	var _avx2 bool
	var _avx512f bool
	var _avx512dq bool
	//	var _avx512pf        bool
	//	var _avx512er        bool
	//	var _avx512cd        bool
	var _avx512bw bool
	var _avx512vl bool
	var _sseState bool
	var _avxState bool
	var _opmaskState bool
	var _zmmHI256State bool
	var _hi16ZmmState bool

	mfi, _, _, _ := cpuid(0)

	if mfi >= 1 {
		_, _, c, d := cpuid(1)

		sse = (d & (1 << 25)) != 0
		sse2 = (d & (1 << 26)) != 0
		sse3 = (c & (1 << 0)) != 0
		ssse3 = (c & (1 << 9)) != 0
		sse41 = (c & (1 << 19)) != 0
		sse42 = (c & (1 << 20)) != 0
		popcnt = (c & (1 << 23)) != 0
		_xsave = (c & (1 << 26)) != 0
		_osxsave = (c & (1 << 27)) != 0
		_avx = (c & (1 << 28)) != 0
	}

	if mfi >= 7 {
		_, b, _, _ := cpuid(7)

		_avx2 = (b & (1 << 5)) != 0
		_avx512f = (b & (1 << 16)) != 0
		_avx512dq = (b & (1 << 17)) != 0
		//		_avx512pf = (b & (1 << 26)) != 0
		//		_avx512er = (b & (1 << 27)) != 0
		//		_avx512cd = (b & (1 << 28)) != 0
		_avx512bw = (b & (1 << 30)) != 0
		_avx512vl = (b & (1 << 31)) != 0
		sha = (b & (1 << 29)) != 0
	}

	// Stop here if XSAVE unsupported or not enabled
	if !_xsave || !_osxsave {
		return
	}

	if _xsave && _osxsave {
		a, _ := xgetbv(0)

		_sseState = (a & (1 << 1)) != 0
		_avxState = (a & (1 << 2)) != 0
		_opmaskState = (a & (1 << 5)) != 0
		_zmmHI256State = (a & (1 << 6)) != 0
		_hi16ZmmState = (a & (1 << 7)) != 0
	} else {
		_sseState = true
	}

	// Very unlikely that OS would enable XSAVE and then disable SSE
	if !_sseState {
		sse = false
		sse2 = false
		sse3 = false
		ssse3 = false
		sse41 = false
		sse42 = false
	}

	if _avxState {
		avx = _avx
		avx2 = _avx2
	}

	if _opmaskState && _zmmHI256State && _hi16ZmmState {
		avx512 = (_avx512f &&
			_avx512dq &&
			_avx512bw &&
			_avx512vl)
	}
}
