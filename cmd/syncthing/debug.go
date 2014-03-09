package main

import (
	"log"
	"os"
	"strings"
)

var (
	dlog      = log.New(os.Stderr, "main: ", log.Lmicroseconds|log.Lshortfile)
	debugNet  = strings.Contains(os.Getenv("STTRACE"), "net")
	debugIdx  = strings.Contains(os.Getenv("STTRACE"), "idx")
	debugNeed = strings.Contains(os.Getenv("STTRACE"), "need")
	debugPull = strings.Contains(os.Getenv("STTRACE"), "pull")
)
