package flags

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"
	"time"
)

func helpDiff(a, b string) (string, error) {
	atmp, err := ioutil.TempFile("", "help-diff")

	if err != nil {
		return "", err
	}

	btmp, err := ioutil.TempFile("", "help-diff")

	if err != nil {
		return "", err
	}

	if _, err := io.WriteString(atmp, a); err != nil {
		return "", err
	}

	if _, err := io.WriteString(btmp, b); err != nil {
		return "", err
	}

	ret, err := exec.Command("diff", "-u", "-d", "--label", "got", atmp.Name(), "--label", "expected", btmp.Name()).Output()

	os.Remove(atmp.Name())
	os.Remove(btmp.Name())

	return string(ret), nil
}

type helpOptions struct {
	Verbose  []bool       `short:"v" long:"verbose" description:"Show verbose debug information" ini-name:"verbose"`
	Call     func(string) `short:"c" description:"Call phone number" ini-name:"call"`
	PtrSlice []*string    `long:"ptrslice" description:"A slice of pointers to string"`

	OnlyIni string `ini-name:"only-ini" description:"Option only available in ini"`

	Other struct {
		StringSlice []string       `short:"s" description:"A slice of strings"`
		IntMap      map[string]int `long:"intmap" description:"A map from string to int" ini-name:"int-map"`
	} `group:"Other Options"`
}

func TestHelp(t *testing.T) {
	var opts helpOptions

	p := NewNamedParser("TestHelp", HelpFlag)
	p.AddGroup("Application Options", "The application options", &opts)

	_, err := p.ParseArgs([]string{"--help"})

	if err == nil {
		t.Fatalf("Expected help error")
	}

	if e, ok := err.(*Error); !ok {
		t.Fatalf("Expected flags.Error, but got %#T", err)
	} else {
		if e.Type != ErrHelp {
			t.Errorf("Expected flags.ErrHelp type, but got %s", e.Type)
		}

		expected := `Usage:
  TestHelp [OPTIONS]

Application Options:
  -v, --verbose   Show verbose debug information
  -c=             Call phone number
      --ptrslice= A slice of pointers to string

Other Options:
  -s=             A slice of strings
      --intmap=   A map from string to int

Help Options:
  -h, --help      Show this help message
`

		if e.Message != expected {
			ret, err := helpDiff(e.Message, expected)

			if err != nil {
				t.Errorf("Unexpected diff error: %s", err)
				t.Errorf("Unexpected help message, expected:\n\n%s\n\nbut got\n\n%s", expected, e.Message)
			} else {
				t.Errorf("Unexpected help message:\n\n%s", ret)
			}
		}
	}
}

func TestMan(t *testing.T) {
	var opts helpOptions

	p := NewNamedParser("TestMan", HelpFlag)
	p.ShortDescription = "Test manpage generation"
	p.LongDescription = "This is a somewhat longer description of what this does"
	p.AddGroup("Application Options", "The application options", &opts)

	var buf bytes.Buffer
	p.WriteManPage(&buf)

	got := buf.String()

	tt := time.Now()

	expected := fmt.Sprintf(`.TH TestMan 1 "%s"
.SH NAME
TestMan \- Test manpage generation
.SH SYNOPSIS
\fBTestMan\fP [OPTIONS]
.SH DESCRIPTION
This is a somewhat longer description of what this does
.SH OPTIONS
.TP
\fB-v, --verbose\fP
Show verbose debug information
.TP
\fB-c\fP
Call phone number
.TP
\fB--ptrslice\fP
A slice of pointers to string
.TP
\fB-s\fP
A slice of strings
.TP
\fB--intmap\fP
A map from string to int
`, tt.Format("2 January 2006"))

	if got != expected {
		ret, err := helpDiff(got, expected)

		if err != nil {
			t.Errorf("Unexpected man page, expected:\n\n%s\n\nbut got\n\n%s", expected, got)
		} else {
			t.Errorf("Unexpected man page:\n\n%s", ret)
		}
	}
}
