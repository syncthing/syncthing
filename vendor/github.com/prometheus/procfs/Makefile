ci: fmt lint test

fmt:
	! gofmt -l *.go | read nothing
	go vet

lint:
	go get github.com/golang/lint/golint
	golint *.go

test: sysfs/fixtures/.unpacked
	go test -v ./...

sysfs/fixtures/.unpacked: sysfs/fixtures.ttar
	./ttar -C sysfs -x -f sysfs/fixtures.ttar
	touch $@

.PHONY: fmt lint test ci
