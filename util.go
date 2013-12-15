package main

import "time"

func timing(name string, t0 time.Time) {
	debugf("%s: %.02f ms", name, time.Since(t0).Seconds()*1000)
}
