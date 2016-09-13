// Copyright 2014 The b Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package b implements the B+tree flavor of a BTree.
//
// Changelog
//
// 2016-07-16: Update benchmark results to newer Go version. Add a note on
// concurrency.
//
// 2014-06-26: Lower GC presure by recycling things.
//
// 2014-04-18: Added new method Put.
//
// Concurrency considerations
//
// Tree.{Clear,Delete,Put,Set} mutate the tree. One can use eg. a
// sync.Mutex.Lock/Unlock (or sync.RWMutex.Lock/Unlock) to wrap those calls if
// they are to be invoked concurrently.
//
// Tree.{First,Get,Last,Len,Seek,SeekFirst,SekLast} read but do not mutate the
// tree.  One can use eg. a sync.RWMutex.RLock/RUnlock to wrap those calls if
// they are to be invoked concurrently with any of the tree mutating methods.
//
// Enumerator.{Next,Prev} mutate the enumerator and read but not mutate the
// tree.  One can use eg. a sync.RWMutex.RLock/RUnlock to wrap those calls if
// they are to be invoked concurrently with any of the tree mutating methods. A
// separate mutex for the enumerator, or the whole tree in a simplified
// variant, is necessary if the enumerator's Next/Prev methods per se are to
// be invoked concurrently.
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
// 3.4GHz, Go 1.7rc1.
//
//	$ go test -bench 1e3 example/all_test.go example/int.go
//	BenchmarkSetSeq1e3-4    	   20000	     78265 ns/op
//	BenchmarkGetSeq1e3-4    	   20000	     67980 ns/op
//	BenchmarkSetRnd1e3-4    	   10000	    172720 ns/op
//	BenchmarkGetRnd1e3-4    	   20000	     89539 ns/op
//	BenchmarkDelSeq1e3-4    	   20000	     87863 ns/op
//	BenchmarkDelRnd1e3-4    	   10000	    130891 ns/op
//	BenchmarkSeekSeq1e3-4   	   10000	    100118 ns/op
//	BenchmarkSeekRnd1e3-4   	   10000	    121684 ns/op
//	BenchmarkNext1e3-4      	  200000	      6330 ns/op
//	BenchmarkPrev1e3-4      	  200000	      9066 ns/op
//	PASS
//	ok  	command-line-arguments	42.531s
//	$
package b
