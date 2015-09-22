# Copyright 2014 The b Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

.PHONY: all todo clean cover generic mem nuke cpu

testbin=b.test

all: editor
	go build
	go vet
	golint .
	go install
	make todo

editor:
	gofmt -l -s -w .
	go test -i
	go test

clean:
	@go clean
	rm -f *~ *.out $(testbin)

cover:
	t=$(shell tempfile) ; go test -coverprofile $$t && go tool cover -html $$t && unlink $$t

cpu:
	go test -c
	./$(testbin) -test.cpuprofile cpu.out
	go tool pprof --lines $(testbin) cpu.out

generic:
	@# writes to stdout a version where the type of key is KEY and the type
	@# of value is VALUE.
	@#
	@# Intended use is to replace all textual occurrences of KEY or VALUE in
	@# the output with your desired types.
	@sed -e 's|interface{}[^{]*/\*K\*/|KEY|g' -e 's|interface{}[^{]*/\*V\*/|VALUE|g' btree.go

mem:
	go test -c
	./$(testbin) -test.bench . -test.memprofile mem.out -test.memprofilerate 1
	go tool pprof --lines --web --alloc_space $(testbin) mem.out

nuke: clean
	rm -f *.test *.out

todo:
	@grep -n ^[[:space:]]*_[[:space:]]*=[[:space:]][[:alpha:]][[:alnum:]]* *.go || true
	@grep -n TODO *.go || true
	@grep -n BUG *.go || true
	@grep -n println *.go || true
