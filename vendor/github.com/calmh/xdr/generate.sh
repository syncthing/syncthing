#!/bin/sh

go run cmd/genxdr/main.go -- bench_test.go > bench_xdr_test.go
go run cmd/genxdr/main.go -- encdec_test.go > encdec_xdr_test.go
