package flags

import (
	"testing"
)

func TestShort(t *testing.T) {
	var opts = struct {
		Value bool `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-v")

	assertStringArray(t, ret, []string{})

	if !opts.Value {
		t.Errorf("Expected Value to be true")
	}
}

func TestShortTooLong(t *testing.T) {
	var opts = struct {
		Value bool `short:"vv"`
	}{}

	assertParseFail(t, ErrShortNameTooLong, "short names can only be 1 character long, not `vv'", &opts)
}

func TestShortRequired(t *testing.T) {
	var opts = struct {
		Value bool `short:"v" required:"true"`
	}{}

	assertParseFail(t, ErrRequired, "the required flag `-v' was not specified", &opts)
}

func TestShortMultiConcat(t *testing.T) {
	var opts = struct {
		V bool `short:"v"`
		O bool `short:"o"`
		F bool `short:"f"`
	}{}

	ret := assertParseSuccess(t, &opts, "-vo", "-f")

	assertStringArray(t, ret, []string{})

	if !opts.V {
		t.Errorf("Expected V to be true")
	}

	if !opts.O {
		t.Errorf("Expected O to be true")
	}

	if !opts.F {
		t.Errorf("Expected F to be true")
	}
}

func TestShortMultiSlice(t *testing.T) {
	var opts = struct {
		Values []bool `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-v", "-v")

	assertStringArray(t, ret, []string{})
	assertBoolArray(t, opts.Values, []bool{true, true})
}

func TestShortMultiSliceConcat(t *testing.T) {
	var opts = struct {
		Values []bool `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-vvv")

	assertStringArray(t, ret, []string{})
	assertBoolArray(t, opts.Values, []bool{true, true, true})
}

func TestShortWithEqualArg(t *testing.T) {
	var opts = struct {
		Value string `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-v=value")

	assertStringArray(t, ret, []string{})
	assertString(t, opts.Value, "value")
}

func TestShortWithArg(t *testing.T) {
	var opts = struct {
		Value string `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-vvalue")

	assertStringArray(t, ret, []string{})
	assertString(t, opts.Value, "value")
}

func TestShortArg(t *testing.T) {
	var opts = struct {
		Value string `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-v", "value")

	assertStringArray(t, ret, []string{})
	assertString(t, opts.Value, "value")
}

func TestShortMultiWithEqualArg(t *testing.T) {
	var opts = struct {
		F     []bool `short:"f"`
		Value string `short:"v"`
	}{}

	assertParseFail(t, ErrExpectedArgument, "expected argument for flag `-v'", &opts, "-ffv=value")
}

func TestShortMultiArg(t *testing.T) {
	var opts = struct {
		F     []bool `short:"f"`
		Value string `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-ffv", "value")

	assertStringArray(t, ret, []string{})
	assertBoolArray(t, opts.F, []bool{true, true})
	assertString(t, opts.Value, "value")
}

func TestShortMultiArgConcatFail(t *testing.T) {
	var opts = struct {
		F     []bool `short:"f"`
		Value string `short:"v"`
	}{}

	assertParseFail(t, ErrExpectedArgument, "expected argument for flag `-v'", &opts, "-ffvvalue")
}

func TestShortMultiArgConcat(t *testing.T) {
	var opts = struct {
		F     []bool `short:"f"`
		Value string `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-vff")

	assertStringArray(t, ret, []string{})
	assertString(t, opts.Value, "ff")
}

func TestShortOptional(t *testing.T) {
	var opts = struct {
		F     []bool `short:"f"`
		Value string `short:"v" optional:"yes" optional-value:"value"`
	}{}

	ret := assertParseSuccess(t, &opts, "-fv", "f")

	assertStringArray(t, ret, []string{"f"})
	assertString(t, opts.Value, "value")
}
