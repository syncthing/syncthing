package main

import (
	"os"
	"strings"
)

var (
	debugNet  = strings.Contains(os.Getenv("STTRACE"), "net") || os.Getenv("STTRACE") == "all"
	debugIdx  = strings.Contains(os.Getenv("STTRACE"), "idx") || os.Getenv("STTRACE") == "all"
	debugNeed = strings.Contains(os.Getenv("STTRACE"), "need") || os.Getenv("STTRACE") == "all"
	debugPull = strings.Contains(os.Getenv("STTRACE"), "pull") || os.Getenv("STTRACE") == "all"
)
