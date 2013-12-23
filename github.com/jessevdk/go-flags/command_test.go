package flags

import (
	"testing"
)

func TestCommandInline(t *testing.T) {
	var opts = struct {
		Value bool `short:"v"`

		Command struct {
			G bool `short:"g"`
		} `command:"cmd"`
	}{}

	p, ret := assertParserSuccess(t, &opts, "-v", "cmd", "-g")

	assertStringArray(t, ret, []string{})

	if p.Active == nil {
		t.Errorf("Expected active command")
	}

	if !opts.Value {
		t.Errorf("Expected Value to be true")
	}

	if !opts.Command.G {
		t.Errorf("Expected Command.G to be true")
	}

	if p.Command.Find("cmd") != p.Active {
		t.Errorf("Expected to find command `cmd' to be active")
	}
}

func TestCommandInlineMulti(t *testing.T) {
	var opts = struct {
		Value bool `short:"v"`

		C1 struct {
		} `command:"c1"`

		C2 struct {
			G bool `short:"g"`
		} `command:"c2"`
	}{}

	p, ret := assertParserSuccess(t, &opts, "-v", "c2", "-g")

	assertStringArray(t, ret, []string{})

	if p.Active == nil {
		t.Errorf("Expected active command")
	}

	if !opts.Value {
		t.Errorf("Expected Value to be true")
	}

	if !opts.C2.G {
		t.Errorf("Expected C2.G to be true")
	}

	if p.Command.Find("c1") == nil {
		t.Errorf("Expected to find command `c1'")
	}

	if c2 := p.Command.Find("c2"); c2 == nil {
		t.Errorf("Expected to find command `c2'")
	} else if c2 != p.Active {
		t.Errorf("Expected to find command `c2' to be active")
	}
}

func TestCommandFlagOrder1(t *testing.T) {
	var opts = struct {
		Value bool `short:"v"`

		Command struct {
			G bool `short:"g"`
		} `command:"cmd"`
	}{}

	assertParseFail(t, ErrUnknownFlag, "unknown flag `g'", &opts, "-v", "-g", "cmd")
}

func TestCommandFlagOrder2(t *testing.T) {
	var opts = struct {
		Value bool `short:"v"`

		Command struct {
			G bool `short:"g"`
		} `command:"cmd"`
	}{}

	assertParseFail(t, ErrUnknownFlag, "unknown flag `v'", &opts, "cmd", "-v", "-g")
}

func TestCommandEstimate(t *testing.T) {
	var opts = struct {
		Value bool `short:"v"`

		Cmd1 struct {
		} `command:"remove"`

		Cmd2 struct {
		} `command:"add"`
	}{}

	p := NewParser(&opts, None)
	_, err := p.ParseArgs([]string{})

	assertError(t, err, ErrRequired, "Please specify one command of: add or remove")
}

type testCommand struct {
	G        bool `short:"g"`
	Executed bool
	EArgs    []string
}

func (c *testCommand) Execute(args []string) error {
	c.Executed = true
	c.EArgs = args

	return nil
}

func TestCommandExecute(t *testing.T) {
	var opts = struct {
		Value bool `short:"v"`

		Command testCommand `command:"cmd"`
	}{}

	assertParseSuccess(t, &opts, "-v", "cmd", "-g", "a", "b")

	if !opts.Value {
		t.Errorf("Expected Value to be true")
	}

	if !opts.Command.Executed {
		t.Errorf("Did not execute command")
	}

	if !opts.Command.G {
		t.Errorf("Expected Command.C to be true")
	}

	assertStringArray(t, opts.Command.EArgs, []string{"a", "b"})
}

func TestCommandClosest(t *testing.T) {
	var opts = struct {
		Value bool `short:"v"`

		Cmd1 struct {
		} `command:"remove"`

		Cmd2 struct {
		} `command:"add"`
	}{}

	assertParseFail(t, ErrRequired, "Unknown command `addd', did you mean `add'?", &opts, "-v", "addd")
}

func TestCommandAdd(t *testing.T) {
	var opts = struct {
		Value bool `short:"v"`
	}{}

	var cmd = struct {
		G bool `short:"g"`
	}{}

	p := NewParser(&opts, Default)
	c, err := p.AddCommand("cmd", "", "", &cmd)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
		return
	}

	ret, err := p.ParseArgs([]string{"-v", "cmd", "-g", "rest"})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
		return
	}

	assertStringArray(t, ret, []string{"rest"})

	if !opts.Value {
		t.Errorf("Expected Value to be true")
	}

	if !cmd.G {
		t.Errorf("Expected Command.G to be true")
	}

	if p.Command.Find("cmd") != c {
		t.Errorf("Expected to find command `cmd'")
	}

	if p.Commands()[0] != c {
		t.Errorf("Espected command #v, but got #v", c, p.Commands()[0])
	}

	if c.Options()[0].ShortName != 'g' {
		t.Errorf("Expected short name `g' but got %v", c.Options()[0].ShortName)
	}
}

func TestCommandNestedInline(t *testing.T) {
	var opts = struct {
		Value bool `short:"v"`

		Command struct {
			G bool `short:"g"`

			Nested struct {
				N string `long:"n"`
			} `command:"nested"`
		} `command:"cmd"`
	}{}

	p, ret := assertParserSuccess(t, &opts, "-v", "cmd", "-g", "nested", "--n", "n", "rest")

	assertStringArray(t, ret, []string{"rest"})

	if !opts.Value {
		t.Errorf("Expected Value to be true")
	}

	if !opts.Command.G {
		t.Errorf("Expected Command.G to be true")
	}

	assertString(t, opts.Command.Nested.N, "n")

	if c := p.Command.Find("cmd"); c == nil {
		t.Errorf("Expected to find command `cmd'")
	} else {
		if c != p.Active {
			t.Errorf("Expected `cmd' to be the active parser command")
		}

		if nested := c.Find("nested"); nested == nil {
			t.Errorf("Expected to find command `nested'")
		} else if nested != c.Active {
			t.Errorf("Expected to find command `nested' to be the active `cmd' command")
		}
	}
}
