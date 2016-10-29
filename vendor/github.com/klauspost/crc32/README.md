# crc32
CRC32 hash with x64 optimizations

This package is a drop-in replacement for the standard library `hash/crc32` package, that features SSE 4.2 optimizations on x64 platforms, for a 10x speedup.

[![Build Status](https://travis-ci.org/klauspost/crc32.svg?branch=master)](https://travis-ci.org/klauspost/crc32)

# usage

Install using `go get github.com/klauspost/crc32`. This library is based on Go 1.5 code and requires Go 1.3 or newer.

Replace `import "hash/crc32"` with `import "github.com/klauspost/crc32"` and you are good to go.

# changes
* Oct 20, 2016: Changes have been merged to upstream Go. Package updated to match.
* Dec 4, 2015: Uses the "slice-by-8" trick more extensively, which gives a 1.5 to 2.5x speedup if assembler is unavailable.


# performance

For *Go 1.7* performance is equivalent to the standard library. So if you use this package for Go 1.7 you can switch back.


For IEEE tables (the most common), there is approximately a factor 10 speedup with "CLMUL" (Carryless multiplication) instruction:
```
benchmark            old ns/op     new ns/op     delta
BenchmarkCrc32KB     99955         10258         -89.74%

benchmark            old MB/s     new MB/s     speedup
BenchmarkCrc32KB     327.83       3194.20      9.74x
```

For other tables and "CLMUL"  capable machines the performance is the same as the standard library.

Here are some detailed benchmarks, comparing to go 1.5 standard library with and without assembler enabled.

```
Std:   Standard Go 1.5 library
Crc:   Indicates IEEE type CRC.
40B:   Size of each slice encoded.
NoAsm: Assembler was disabled (ie. not an AMD64 or SSE 4.2+ capable machine).
Castagnoli: Castagnoli CRC type.

BenchmarkStdCrc40B-4            10000000               158 ns/op         252.88 MB/s
BenchmarkCrc40BNoAsm-4          20000000               105 ns/op         377.38 MB/s (slice8)
BenchmarkCrc40B-4               20000000               105 ns/op         378.77 MB/s (slice8)

BenchmarkStdCrc1KB-4              500000              3604 ns/op         284.10 MB/s
BenchmarkCrc1KBNoAsm-4           1000000              1463 ns/op         699.79 MB/s (slice8)
BenchmarkCrc1KB-4                3000000               396 ns/op        2583.69 MB/s (asm)

BenchmarkStdCrc8KB-4              200000             11417 ns/op         717.48 MB/s (slice8)
BenchmarkCrc8KBNoAsm-4            200000             11317 ns/op         723.85 MB/s (slice8)
BenchmarkCrc8KB-4                 500000              2919 ns/op        2805.73 MB/s (asm)

BenchmarkStdCrc32KB-4              30000             45749 ns/op         716.24 MB/s (slice8)
BenchmarkCrc32KBNoAsm-4            30000             45109 ns/op         726.42 MB/s (slice8)
BenchmarkCrc32KB-4                100000             11497 ns/op        2850.09 MB/s (asm)

BenchmarkStdNoAsmCastagnol40B-4 10000000               161 ns/op         246.94 MB/s
BenchmarkStdCastagnoli40B-4     50000000              28.4 ns/op        1410.69 MB/s (asm)
BenchmarkCastagnoli40BNoAsm-4   20000000               100 ns/op         398.01 MB/s (slice8)
BenchmarkCastagnoli40B-4        50000000              28.2 ns/op        1419.54 MB/s (asm)

BenchmarkStdNoAsmCastagnoli1KB-4  500000              3622 ns/op        282.67 MB/s
BenchmarkStdCastagnoli1KB-4     10000000               144 ns/op        7099.78 MB/s (asm)
BenchmarkCastagnoli1KBNoAsm-4    1000000              1475 ns/op         694.14 MB/s (slice8)
BenchmarkCastagnoli1KB-4        10000000               146 ns/op        6993.35 MB/s (asm)

BenchmarkStdNoAsmCastagnoli8KB-4  50000              28781 ns/op         284.63 MB/s
BenchmarkStdCastagnoli8KB-4      1000000              1029 ns/op        7957.89 MB/s (asm)
BenchmarkCastagnoli8KBNoAsm-4     200000             11410 ns/op         717.94 MB/s (slice8)
BenchmarkCastagnoli8KB-4         1000000              1000 ns/op        8188.71 MB/s (asm)

BenchmarkStdNoAsmCastagnoli32KB-4  10000            115426 ns/op         283.89 MB/s
BenchmarkStdCastagnoli32KB-4      300000              4065 ns/op        8059.13 MB/s (asm)
BenchmarkCastagnoli32KBNoAsm-4     30000             45171 ns/op         725.41 MB/s (slice8)
BenchmarkCastagnoli32KB-4         500000              4077 ns/op        8035.89 MB/s (asm)
```

The IEEE assembler optimizations has been submitted and will be part of the Go 1.6 standard library.

However, the improved use of slice-by-8 has not, but will probably be submitted for Go 1.7.

# license

Standard Go license. Changes are Copyright (c) 2015 Klaus Post under same conditions.
