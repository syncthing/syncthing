murmur3
=======

Native Go implementation of Austin Appleby's third MurmurHash revision (aka
MurmurHash3).

Reference algorithm has been slightly hacked as to support the streaming mode
required by Go's standard [Hash interface](http://golang.org/pkg/hash/#Hash).


Benchmarks
----------

Go tip as of 2014-06-12 (i.e almost go1.3), core i7 @ 3.4 Ghz. All runs
include hasher instantiation and sequence finalization.

<pre>

Benchmark32_1        500000000     7.69 ns/op      130.00 MB/s
Benchmark32_2        200000000     8.83 ns/op      226.42 MB/s
Benchmark32_4        500000000     7.99 ns/op      500.39 MB/s
Benchmark32_8        200000000     9.47 ns/op      844.69 MB/s
Benchmark32_16       100000000     12.1 ns/op     1321.61 MB/s
Benchmark32_32       100000000     18.3 ns/op     1743.93 MB/s
Benchmark32_64        50000000     30.9 ns/op     2071.64 MB/s
Benchmark32_128       50000000     57.6 ns/op     2222.96 MB/s
Benchmark32_256       20000000      116 ns/op     2188.60 MB/s
Benchmark32_512       10000000      226 ns/op     2260.59 MB/s
Benchmark32_1024       5000000      452 ns/op     2263.73 MB/s
Benchmark32_2048       2000000      891 ns/op     2296.02 MB/s
Benchmark32_4096       1000000     1787 ns/op     2290.92 MB/s
Benchmark32_8192        500000     3593 ns/op     2279.68 MB/s
Benchmark128_1       100000000     26.1 ns/op       38.33 MB/s
Benchmark128_2       100000000     29.0 ns/op       69.07 MB/s
Benchmark128_4        50000000     29.8 ns/op      134.17 MB/s
Benchmark128_8        50000000     31.6 ns/op      252.86 MB/s
Benchmark128_16      100000000     26.5 ns/op      603.42 MB/s
Benchmark128_32      100000000     28.6 ns/op     1117.15 MB/s
Benchmark128_64       50000000     35.5 ns/op     1800.97 MB/s
Benchmark128_128      50000000     50.9 ns/op     2515.50 MB/s
Benchmark128_256      20000000     76.9 ns/op     3330.11 MB/s
Benchmark128_512      20000000      135 ns/op     3769.09 MB/s
Benchmark128_1024     10000000      250 ns/op     4094.38 MB/s
Benchmark128_2048      5000000      477 ns/op     4290.75 MB/s
Benchmark128_4096      2000000      940 ns/op     4353.29 MB/s
Benchmark128_8192      1000000     1838 ns/op     4455.47 MB/s

</pre>


<pre>

benchmark              Go1.0 MB/s    Go1.1 MB/s  speedup    Go1.2 MB/s  speedup    Go1.3 MB/s  speedup
Benchmark32_1               98.90        118.59    1.20x        114.79    0.97x        130.00    1.13x
Benchmark32_2              168.04        213.31    1.27x        210.65    0.99x        226.42    1.07x
Benchmark32_4              414.01        494.19    1.19x        490.29    0.99x        500.39    1.02x
Benchmark32_8              662.19        836.09    1.26x        836.46    1.00x        844.69    1.01x
Benchmark32_16             917.46       1304.62    1.42x       1297.63    0.99x       1321.61    1.02x
Benchmark32_32            1141.93       1737.54    1.52x       1728.24    0.99x       1743.93    1.01x
Benchmark32_64            1289.47       2039.51    1.58x       2038.20    1.00x       2071.64    1.02x
Benchmark32_128           1299.23       2097.63    1.61x       2177.13    1.04x       2222.96    1.02x
Benchmark32_256           1369.90       2202.34    1.61x       2213.15    1.00x       2188.60    0.99x
Benchmark32_512           1399.56       2255.72    1.61x       2264.49    1.00x       2260.59    1.00x
Benchmark32_1024          1410.90       2285.82    1.62x       2270.99    0.99x       2263.73    1.00x
Benchmark32_2048          1422.14       2297.62    1.62x       2269.59    0.99x       2296.02    1.01x
Benchmark32_4096          1420.53       2307.81    1.62x       2273.43    0.99x       2290.92    1.01x
Benchmark32_8192          1424.79       2312.87    1.62x       2286.07    0.99x       2279.68    1.00x
Benchmark128_1               8.32         30.15    3.62x         30.84    1.02x         38.33    1.24x
Benchmark128_2              16.38         59.72    3.65x         59.37    0.99x         69.07    1.16x
Benchmark128_4              32.26        112.96    3.50x        114.24    1.01x        134.17    1.17x
Benchmark128_8              62.68        217.88    3.48x        218.18    1.00x        252.86    1.16x
Benchmark128_16            128.47        451.57    3.51x        474.65    1.05x        603.42    1.27x
Benchmark128_32            246.18        910.42    3.70x        871.06    0.96x       1117.15    1.28x
Benchmark128_64            449.05       1477.64    3.29x       1449.24    0.98x       1800.97    1.24x
Benchmark128_128           762.61       2222.42    2.91x       2217.30    1.00x       2515.50    1.13x
Benchmark128_256          1179.92       3005.46    2.55x       2931.55    0.98x       3330.11    1.14x
Benchmark128_512          1616.51       3590.75    2.22x       3592.08    1.00x       3769.09    1.05x
Benchmark128_1024         1964.36       3979.67    2.03x       4034.01    1.01x       4094.38    1.01x
Benchmark128_2048         2225.07       4156.93    1.87x       4244.17    1.02x       4290.75    1.01x
Benchmark128_4096         2360.15       4299.09    1.82x       4392.35    1.02x       4353.29    0.99x
Benchmark128_8192         2411.50       4356.84    1.81x       4480.68    1.03x       4455.47    0.99x

</pre>
