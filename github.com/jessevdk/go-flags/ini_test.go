package flags

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteIni(t *testing.T) {
	var opts helpOptions

	p := NewNamedParser("TestIni", Default)
	p.AddGroup("Application Options", "The application options", &opts)

	p.ParseArgs([]string{"-vv", "--intmap=a:2", "--intmap", "b:3"})

	inip := NewIniParser(p)

	var b bytes.Buffer
	inip.Write(&b, IniDefault|IniIncludeDefaults)

	got := b.String()
	expected := `[Application Options]
; Show verbose debug information
verbose = true
verbose = true

; A slice of pointers to string
; PtrSlice =

; Option only available in ini
only-ini =

[Other Options]
; A slice of strings
; StringSlice =

; A map from string to int
int-map = a:2
int-map = b:3

`

	if got != expected {
		ret, err := helpDiff(got, expected)

		if err != nil {
			t.Errorf("Unexpected ini, expected:\n\n%s\n\nbut got\n\n%s", expected, got)
		} else {
			t.Errorf("Unexpected ini:\n\n%s", ret)
		}
	}
}

func TestReadIni(t *testing.T) {
	var opts helpOptions

	p := NewNamedParser("TestIni", Default)
	p.AddGroup("Application Options", "The application options", &opts)

	inip := NewIniParser(p)

	inic := `
; Show verbose debug information
verbose = true
verbose = true

[Application Options]
; A slice of pointers to string
; PtrSlice =

[Other Options]
; A slice of strings
; StringSlice =

; A map from string to int
int-map = a:2
int-map = b:3

`

	b := strings.NewReader(inic)
	err := inip.Parse(b)

	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	assertBoolArray(t, opts.Verbose, []bool{true, true})

	if v, ok := opts.Other.IntMap["a"]; !ok {
		t.Errorf("Expected \"a\" in Other.IntMap")
	} else if v != 2 {
		t.Errorf("Expected Other.IntMap[\"a\"] = 2, but got %v", v)
	}

	if v, ok := opts.Other.IntMap["b"]; !ok {
		t.Errorf("Expected \"b\" in Other.IntMap")
	} else if v != 3 {
		t.Errorf("Expected Other.IntMap[\"b\"] = 3, but got %v", v)
	}
}

func TestIniCommands(t *testing.T) {
	var opts struct {
		Value string `short:"v" long:"value"`

		Add struct {
			Name int `short:"n" long:"name" ini-name:"AliasName"`

			Other struct {
				O string `short:"o" long:"other"`
			} `group:"Other Options"`
		} `command:"add"`
	}

	p := NewNamedParser("TestIni", Default)
	p.AddGroup("Application Options", "The application options", &opts)

	inip := NewIniParser(p)

	inic := `[Application Options]
value = some value

[add]
AliasName = 5

[add.Other Options]
other = subgroup
`

	b := strings.NewReader(inic)
	err := inip.Parse(b)

	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	assertString(t, opts.Value, "some value")

	if opts.Add.Name != 5 {
		t.Errorf("Expected opts.Add.Name to be 5, but got %v", opts.Add.Name)
	}

	assertString(t, opts.Add.Other.O, "subgroup")
}

func TestIniNoIni(t *testing.T) {
	var opts struct {
		Value string `short:"v" long:"value" no-ini:"yes"`
	}

	p := NewNamedParser("TestIni", Default)
	p.AddGroup("Application Options", "The application options", &opts)

	inip := NewIniParser(p)

	inic := `[Application Options]
value = some value
`

	b := strings.NewReader(inic)
	err := inip.Parse(b)

	if err == nil {
		t.Fatalf("Expected error")
	}

	assertError(t, err, ErrUnknownFlag, "unknown option: value")
}
