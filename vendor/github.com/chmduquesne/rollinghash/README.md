[![Build Status](https://travis-ci.org/chmduquesne/rollinghash.svg?branch=master)](https://travis-ci.org/chmduquesne/rollinghash)
[![Coverage Status](https://coveralls.io/repos/github/chmduquesne/rollinghash/badge.svg?branch=master)](https://coveralls.io/github/chmduquesne/rollinghash?branch=master)
[![GoDoc Reference](http://godoc.org/github.com/chmduquesne/rollinghash?status.svg)](https://godoc.org/github.com/chmduquesne/rollinghash)
![Go 1.7+](https://img.shields.io/badge/go-1.7%2B-orange.svg)

Rolling Hashes
==============

Philosophy
----------

This package contains several various rolling hashes for you to play with
crazy ideas. The API design philosophy is to stick as closely as possible
to the interface provided by the builtin hash package (the hashes
implemented here are effectively drop-in replacements for their builtin
counterparts), while providing simultaneously the highest speed and
simplicity.

Usage
-----

A [`rollinghash.Hash`](https://godoc.org/github.com/chmduquesne/rollinghash#Hash)
is just a [`hash.Hash`](https://golang.org/pkg/hash/#Hash) which
implements the
[`Roller`](https://godoc.org/github.com/chmduquesne/rollinghash#Roller)
interface. Here is how it is typically used:

```golang
data := []byte("here is some data to roll on")
h := buzhash64.New()
n := 16

// Initialize the rolling window
h.Write(data[:n])

for _, c := range(data[n:]) {

    // Slide the window and update the hash
    h.Roll(c)

    // Get the updated hash value
    fmt.Println(h.Sum64())
}
```

Gotchas
-------

The rolling window MUST be initialized by calling `Write` first (which
saves a copy). The byte leaving the rolling window is inferred from the
internal copy of the rolling window, which is updated with every call to
`Roll`.

If you want your code to run at the highest speed, do NOT cast the result
of a `New()` as a rollinghash.Hash. Instead, use the native type returned
by `New()`. This is because the go compiler cannot inline calls from an
interface. When later you call Roll(), the native type call will be
inlined by the compiler, but not the casted type call.

```golang
var h1 rollinghash.Hash
h1 = buzhash32.New()
h2 := buzhash32.New()

[...]

h1.Roll(b) // Not inlined (slow)
h2.Roll(b) // inlined (fast)
```

What's new in v4
----------------

In v4:

* `Write` has become fully consistent with `hash.Hash`. As opposed to
  previous versions, where writing data would reinitialize the window, it
  now appends this data to the existing window. In order to reset the
  window, one should instead use the `Reset` method.

* Calling `Roll` on an empty window is considered a bug, and now triggers
  a panic.

Brief reminder of the behaviors in previous versions:

* From v0.x.x to v2.x.x: `Roll` returns an error for an empty window.
  `Write` reinitializes the rolling window.

* v3.x.x : `Roll` does not return anything. `Write` still reinitializes
  the rolling window. The rolling window always has a minimum size of 1,
  which yields wrong results when using roll before having initialized the
  window.

Go versions
-----------

The `RabinKarp64` rollinghash does not yield consistent results before
go1.7. This is because it uses `Rand.Read()` from the builtin `math/rand`.
This function was [fixed in go
1.7](https://golang.org/doc/go1.7#math_rand) to produce a consistent
stream of bytes that is independant of the size of the input buffer. If
you depend on this hash, it is strongly recommended to stick to versions
of go superior to 1.7.

License
-------

This code is delivered to you under the terms of the MIT public license,
except the `rabinkarp64` subpackage, which has been adapted from
[restic](https://github.com/restic/chunker) (BSD 2-clause "Simplified").

Notable users
-------------

* [syncthing](https://syncthing.net/), a decentralized synchronisation
  solution
* [muscato](https://github.com/kshedden/muscato), a genome analysis tool

If you are using this in production or for research, let me know and I
will happily put a link here!
