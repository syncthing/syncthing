// Copyright 2014 The b Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package b implements the B+tree flavor of a BTree.
//
// Changelog
//
// 2014-06-26: Lower GC presure by recycling things.
//
// 2014-04-18: Added new method Put.
//
// Generic types
//
// Keys and their associated values are interface{} typed, similar to all of
// the containers in the standard library.
//
// Semiautomatic production of a type specific variant of this package is
// supported via
//
//	$ make generic
//
// This command will write to stdout a version of the btree.go file where every
// key type occurrence is replaced by the word 'KEY' and every value type
// occurrence is replaced by the word 'VALUE'. Then you have to replace these
// tokens with your desired type(s), using any technique you're comfortable
// with.
//
// This is how, for example, 'example/int.go' was created:
//
//	$ mkdir example
//	$ make generic | sed -e 's/KEY/int/g' -e 's/VALUE/int/g' > example/int.go
//
// No other changes to int.go are necessary, it compiles just fine.
//
// Running the benchmarks for 1000 keys on a machine with Intel i5-4670 CPU @
// 3.4GHz, Go release 1.4.2.
//
//	$ go test -bench 1e3 example/all_test.go example/int.go
//	PASS
//	BenchmarkSetSeq1e3	   10000	    151620 ns/op
//	BenchmarkGetSeq1e3	   10000	    115354 ns/op
//	BenchmarkSetRnd1e3	    5000	    255865 ns/op
//	BenchmarkGetRnd1e3	   10000	    140466 ns/op
//	BenchmarkDelSeq1e3	   10000	    143860 ns/op
//	BenchmarkDelRnd1e3	   10000	    188228 ns/op
//	BenchmarkSeekSeq1e3	   10000	    156448 ns/op
//	BenchmarkSeekRnd1e3	   10000	    190587 ns/op
//	BenchmarkNext1e3	  200000	      9407 ns/op
//	BenchmarkPrev1e3	  200000	      9306 ns/op
//	ok  	command-line-arguments	26.369s
//	$
package b
