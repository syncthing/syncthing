package flags

import (
	"testing"
)

func TestLong(t *testing.T) {
	var opts = struct {
		Value bool `long:"value"`
	}{}

	ret := assertParseSuccess(t, &opts, "--value")

	assertStringArray(t, ret, []string{})

	if !opts.Value {
		t.Errorf("Expected Value to be true")
	}
}

func TestLongArg(t *testing.T) {
	var opts = struct {
		Value string `long:"value"`
	}{}

	ret := assertParseSuccess(t, &opts, "--value", "value")

	assertStringArray(t, ret, []string{})
	assertString(t, opts.Value, "value")
}

func TestLongArgEqual(t *testing.T) {
	var opts = struct {
		Value string `long:"value"`
	}{}

	ret := assertParseSuccess(t, &opts, "--value=value")

	assertStringArray(t, ret, []string{})
	assertString(t, opts.Value, "value")
}

func TestLongDefault(t *testing.T) {
	var opts = struct {
		Value string `long:"value" default:"value"`
	}{}

	ret := assertParseSuccess(t, &opts)

	assertStringArray(t, ret, []string{})
	assertString(t, opts.Value, "value")
}

func TestLongOptional(t *testing.T) {
	var opts = struct {
		Value string `long:"value" optional:"yes" optional-value:"value"`
	}{}

	ret := assertParseSuccess(t, &opts, "--value")

	assertStringArray(t, ret, []string{})
	assertString(t, opts.Value, "value")
}

func TestLongOptionalArg(t *testing.T) {
	var opts = struct {
		Value string `long:"value" optional:"yes" optional-value:"value"`
	}{}

	ret := assertParseSuccess(t, &opts, "--value", "no")

	assertStringArray(t, ret, []string{"no"})
	assertString(t, opts.Value, "value")
}

func TestLongOptionalArgEqual(t *testing.T) {
	var opts = struct {
		Value string `long:"value" optional:"yes" optional-value:"value"`
	}{}

	ret := assertParseSuccess(t, &opts, "--value=value", "no")

	assertStringArray(t, ret, []string{"no"})
	assertString(t, opts.Value, "value")
}
