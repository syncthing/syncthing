go-lz4
======

go-lz4 is port of LZ4 lossless compression algorithm to Go. The original C code
is located at:

https://github.com/Cyan4973/lz4

Status
------
[![Build Status](https://secure.travis-ci.org/bkaradzic/go-lz4.png)](http://travis-ci.org/bkaradzic/go-lz4)  
[![GoDoc](https://godoc.org/github.com/bkaradzic/go-lz4?status.png)](https://godoc.org/github.com/bkaradzic/go-lz4)

Usage
-----

    go get github.com/bkaradzic/go-lz4

    import "github.com/bkaradzic/go-lz4"

The package name is `lz4`

Notes
-----

* go-lz4 saves a uint32 with the original uncompressed length at the beginning
  of the encoded buffer.  They may get in the way of interoperability with
  other implementations.

Alternative
-----------

https://github.com/pierrec/lz4

Contributors
------------

Damian Gryski ([@dgryski](https://github.com/dgryski))  
Dustin Sallings ([@dustin](https://github.com/dustin))

Contact
-------

[@bkaradzic](https://twitter.com/bkaradzic)  
http://www.stuckingeometry.com

Project page  
https://github.com/bkaradzic/go-lz4

License
-------

Copyright 2011-2012 Branimir Karadzic. All rights reserved.  
Copyright 2013 Damian Gryski. All rights reserved.

Redistribution and use in source and binary forms, with or without modification,
are permitted provided that the following conditions are met:

   1. Redistributions of source code must retain the above copyright notice, this
      list of conditions and the following disclaimer.

   2. Redistributions in binary form must reproduce the above copyright notice,
      this list of conditions and the following disclaimer in the documentation
      and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY COPYRIGHT HOLDER ``AS IS'' AND ANY EXPRESS OR
IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT
SHALL COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT,
INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR
PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY,
WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE
OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF
THE POSSIBILITY OF SUCH DAMAGE.

