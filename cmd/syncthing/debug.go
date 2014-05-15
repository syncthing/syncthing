package main

import (
	"os"
	"strings"
)

var (
	debugNet = strings.Contains(os.Getenv("STTRACE"), "net") || os.Getenv("STTRACE") == "all"
)
