package flags

import (
	"testing"
)

func TestPointerBool(t *testing.T) {
	var opts = struct {
		Value *bool `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-v")

	assertStringArray(t, ret, []string{})

	if !*opts.Value {
		t.Errorf("Expected Value to be true")
	}
}

func TestPointerString(t *testing.T) {
	var opts = struct {
		Value *string `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-v", "value")

	assertStringArray(t, ret, []string{})
	assertString(t, *opts.Value, "value")
}

func TestPointerSlice(t *testing.T) {
	var opts = struct {
		Value *[]string `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-v", "value1", "-v", "value2")

	assertStringArray(t, ret, []string{})
	assertStringArray(t, *opts.Value, []string{"value1", "value2"})
}

func TestPointerMap(t *testing.T) {
	var opts = struct {
		Value *map[string]int `short:"v"`
	}{}

	ret := assertParseSuccess(t, &opts, "-v", "k1:2", "-v", "k2:-5")

	assertStringArray(t, ret, []string{})

	if v, ok := (*opts.Value)["k1"]; !ok {
		t.Errorf("Expected key \"k1\" to exist")
	} else if v != 2 {
		t.Errorf("Expected \"k1\" to be 2, but got %#v", v)
	}

	if v, ok := (*opts.Value)["k2"]; !ok {
		t.Errorf("Expected key \"k2\" to exist")
	} else if v != -5 {
		t.Errorf("Expected \"k2\" to be -5, but got %#v", v)
	}
}

type PointerGroup struct {
	Value bool `short:"v"`
}

func TestPointerGroup(t *testing.T) {
	var opts = struct {
		Group *PointerGroup `group:"Group Options"`
	}{}

	ret := assertParseSuccess(t, &opts, "-v")

	assertStringArray(t, ret, []string{})

	if !opts.Group.Value {
		t.Errorf("Expected Group.Value to be true")
	}
}
