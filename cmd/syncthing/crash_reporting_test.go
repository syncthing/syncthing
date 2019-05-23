package main

import (
	"bytes"
	"testing"
)

func TestFilterLogLines(t *testing.T) {
	in := []byte(`[ABCD123] syncthing version whatever
here is more log data
and more
...
and some more
yet more
Panic detected at like right now
here is panic data
and yet more panic stuff
`)

	filtered := []byte(`syncthing version whatever
Panic detected at like right now
here is panic data
and yet more panic stuff
`)

	result := filterLogLines(in)
	if !bytes.Equal(result, filtered) {
		t.Logf("%q\n", result)
		t.Error("it should have been filtered")
	}
}
