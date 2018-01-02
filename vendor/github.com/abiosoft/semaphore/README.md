
semaphore
=========

Semaphore implementation in golang

[![Build Status](https://travis-ci.org/abiosoft/semaphore.svg?branch=master)](https://travis-ci.org/abiosoft/semaphore)
[![GoDoc](https://godoc.org/github.com/abiosoft/semaphore?status.svg)](https://godoc.org/github.com/abiosoft/semaphore)
[![Go Report Card](https://goreportcard.com/badge/github.com/abiosoft/semaphore)](https://goreportcard.com/report/github.com/abiosoft/semaphore)

### Usage
Initiate
```go
import "github.com/abiosoft/semaphore"
...
sem := semaphore.New(5) // new semaphore with 5 permits
```
Acquire
```go
sem.Acquire() // one
sem.AcquireMany(n) // multiple
sem.AcquireWithin(n, time.Second * 5) // timeout after 5 sec
sem.AcquireContext(ctx, n) // acquire with context
```
Release
```go
sem.Release() // one
sem.ReleaseMany(n) // multiple
```

### documentation

http://godoc.org/github.com/abiosoft/semaphore
