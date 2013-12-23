package flags

import (
	"fmt"
	"testing"
)

type marshalled bool

func (m *marshalled) UnmarshalFlag(value string) error {
	if value == "yes" {
		*m = true
	} else if value == "no" {
		*m = false
	} else {
		return fmt.Errorf("`%s' is not a valid value, please specify `yes' or `no'", value)
	}

	return nil
}

func (m marshalled) MarshalFlag() string {
	if m {
		return "yes"
	}

	return "no"
}

func TestMarshal(t *testing.T) {
	var opts = struct {
		Value marshalled `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-v=yes")

	assertStringArray(t, ret, []string{})

	if !opts.Value {
		t.Errorf("Expected Value to be true")
	}
}

func TestMarshalDefault(t *testing.T) {
	var opts = struct {
		Value marshalled `short:"v" default:"yes"`
	}{}

	ret := assertParseSuccess(t, &opts)

	assertStringArray(t, ret, []string{})

	if !opts.Value {
		t.Errorf("Expected Value to be true")
	}
}

func TestMarshalOptional(t *testing.T) {
	var opts = struct {
		Value marshalled `short:"v" optional:"yes" optional-value:"yes"`
	}{}

	ret := assertParseSuccess(t, &opts, "-v")

	assertStringArray(t, ret, []string{})

	if !opts.Value {
		t.Errorf("Expected Value to be true")
	}
}

func TestMarshalError(t *testing.T) {
	var opts = struct {
		Value marshalled `short:"v"`
	}{}

	assertParseFail(t, ErrMarshal, "invalid argument for flag `-v' (expected flags.marshalled): `invalid' is not a valid value, please specify `yes' or `no'", &opts, "-vinvalid")
}
