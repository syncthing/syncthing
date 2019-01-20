# sha256-simd

Accelerate SHA256 computations in pure Go using AVX512, SHA Extensions and AVX2 for Intel and ARM64 for ARM. On AVX512 it provides an up to 8x improvement (over 3 GB/s per core) in comparison to AVX2. SHA Extensions give a performance boost of close to 4x over AVX2.

## Introduction

This package is designed as a replacement for `crypto/sha256`. For Intel CPUs it has two flavors for AVX512 and AVX2 (AVX/SSE are also supported). For ARM CPUs with the Cryptography Extensions, advantage is taken of the SHA2 instructions resulting in a massive performance improvement.

This package uses Golang assembly. The AVX512 version is based on the Intel's "multi-buffer crypto library for IPSec" whereas the other Intel implementations are described in "Fast SHA-256 Implementations on Intel Architecture Processors" by J. Guilford et al.

## New: Support for Intel SHA Extensions

Support for the Intel SHA Extensions has been added by Kristofer Peterson (@svenski123), originally developed for spacemeshos [here](https://github.com/spacemeshos/POET/issues/23). On CPUs that support it (known thus far Intel Celeron J3455 and AMD Ryzen) it gives a significant boost in performance (with thanks to @AudriusButkevicius for reporting the results; full results [here](https://github.com/minio/sha256-simd/pull/37#issuecomment-451607827)).

```
$ benchcmp avx2.txt sha-ext.txt
benchmark           AVX2 MB/s    SHA Ext MB/s  speedup
BenchmarkHash5M     514.40       1975.17       3.84x
```

Thanks to Kristofer Peterson, we also added additional performance changes such as optimized padding, endian conversions which sped up all implementations i.e. Intel SHA alone while doubled performance for small sizes, the other changes increased everything roughly 50%.

## Support for AVX512

We have added support for AVX512 which results in an up to 8x performance improvement over AVX2 (3.0 GHz Xeon Platinum 8124M CPU):

```
$ benchcmp avx2.txt avx512.txt
benchmark           AVX2 MB/s    AVX512 MB/s  speedup
BenchmarkHash5M     448.62       3498.20      7.80x
```

The original code was developed by Intel as part of the [multi-buffer crypto library](https://github.com/intel/intel-ipsec-mb) for IPSec or more specifically this [AVX512](https://github.com/intel/intel-ipsec-mb/blob/master/avx512/sha256_x16_avx512.asm) implementation. The key idea behind it is to process a total of 16 checksums in parallel by “transposing” 16 (independent) messages of 64 bytes between a total of 16 ZMM registers (each 64 bytes wide).

Transposing the input messages means that in order to take full advantage of the speedup you need to have a (server) workload where multiple threads are doing SHA256 calculations in parallel. Unfortunately for this algorithm it is not possible for two message blocks processed in parallel to be dependent on one another — because then the (interim) result of the first part of the message has to be an input into the processing of the second part of the message.

Whereas the original Intel C implementation requires some sort of explicit scheduling of messages to be processed in parallel, for Golang it makes sense to take advantage of channels in order to group messages together and use channels as well for sending back the results (thereby effectively decoupling the calculations). We have implemented a fairly simple scheduling mechanism that seems to work well in practice.

Due to this different way of scheduling, we decided to use an explicit method to instantiate the AVX512 version. Essentially one or more AVX512 processing servers ([`Avx512Server`](https://github.com/minio/sha256-simd/blob/master/sha256blockAvx512_amd64.go#L294)) have to be created whereby each server can hash over 3 GB/s on a single core. An `hash.Hash` object ([`Avx512Digest`](https://github.com/minio/sha256-simd/blob/master/sha256blockAvx512_amd64.go#L45)) is then instantiated using one of these servers and used in the regular fashion:

```go
import "github.com/minio/sha256-simd"

func main() {
	server := sha256.NewAvx512Server()
	h512 := sha256.NewAvx512(server)
	h512.Write(fileBlock)
	digest := h512.Sum([]byte{})
}
```

Note that, because of the scheduling overhead, for small messages (< 1 MB) you will be better off using the regular SHA256 hashing (but those are typically not performance critical anyway). Some other tips to get the best performance:
* Have many go routines doing SHA256 calculations in parallel.
* Try to Write() messages in multiples of 64 bytes.
* Try to keep the overall length of messages to a roughly similar size ie. 5 MB (this way all 16 ‘lanes’ in the AVX512 computations are contributing as much as possible).

More detailed information can be found in this [blog](https://blog.minio.io/accelerate-sha256-up-to-8x-over-3-gb-s-per-core-with-avx512-a0b1d64f78f) post including scaling across cores.

## Drop-In Replacement

The following code snippet shows how you can use `github.com/minio/sha256-simd`. This will automatically select the fastest method for the architecture on which it will be executed.

```go
import "github.com/minio/sha256-simd"

func main() {
        ...
	shaWriter := sha256.New()
	io.Copy(shaWriter, file)
        ...
}
```

## Performance

Below is the speed in MB/s for a single core (ranked fast to slow) for blocks larger than 1 MB.

| Processor                         | SIMD    | Speed (MB/s) |
| --------------------------------- | ------- | ------------:|
| 3.0 GHz Intel Xeon Platinum 8124M | AVX512  |         3498 |
| 3.7 GHz AMD Ryzen 7 2700X         | SHA Ext |         1979 |
| 1.2 GHz ARM Cortex-A53            | ARM64   |          638 |
| 3.0 GHz Intel Xeon Platinum 8124M | AVX2    |          449 |
| 3.1 GHz Intel Core i7             | AVX     |          362 |
| 3.1 GHz Intel Core i7             | SSE     |          299 |

## asm2plan9s

In order to be able to work more easily with AVX512/AVX2 instructions, a separate tool was developed to convert SIMD instructions into the corresponding BYTE sequence as accepted by Go assembly. See [asm2plan9s](https://github.com/minio/asm2plan9s) for more information.

## Why and benefits

One of the most performance sensitive parts of the [Minio](https://github.com/minio/minio) object storage server is related to SHA256 hash sums calculations. For instance during multi part uploads each part that is uploaded needs to be verified for data integrity by the server.

Other applications that can benefit from enhanced SHA256 performance are deduplication in storage systems, intrusion detection, version control systems, integrity checking, etc.

## ARM SHA Extensions

The 64-bit ARMv8 core has introduced new instructions for SHA1 and SHA2 acceleration as part of the [Cryptography Extensions](http://infocenter.arm.com/help/index.jsp?topic=/com.arm.doc.ddi0501f/CHDFJBCJ.html). Below you can see a small excerpt highlighting one of the rounds as is done for the SHA256 calculation process (for full code see [sha256block_arm64.s](https://github.com/minio/sha256-simd/blob/master/sha256block_arm64.s)).

 ```
 sha256h    q2, q3, v9.4s
 sha256h2   q3, q4, v9.4s
 sha256su0  v5.4s, v6.4s
 rev32      v8.16b, v8.16b
 add        v9.4s, v7.4s, v18.4s
 mov        v4.16b, v2.16b
 sha256h    q2, q3, v10.4s
 sha256h2   q3, q4, v10.4s
 sha256su0  v6.4s, v7.4s
 sha256su1  v5.4s, v7.4s, v8.4s
 ```

### Detailed benchmarks

Benchmarks generated on a 1.2 Ghz Quad-Core ARM Cortex A53 equipped [Pine64](https://www.pine64.com/).

```
minio@minio-arm:$ benchcmp golang.txt arm64.txt
benchmark                 golang         arm64        speedup
BenchmarkHash8Bytes-4     0.68 MB/s      5.70 MB/s      8.38x
BenchmarkHash1K-4         5.65 MB/s    326.30 MB/s     57.75x
BenchmarkHash8K-4         6.00 MB/s    570.63 MB/s     95.11x
BenchmarkHash1M-4         6.05 MB/s    638.23 MB/s    105.49x
```

## License

Released under the Apache License v2.0. You can find the complete text in the file LICENSE.

## Contributing

Contributions are welcome, please send PRs for any enhancements.
