package ini_test

import (
	"bytes"
	"github.com/calmh/ini"
	"strings"
	"testing"
)

func TestParseValues(t *testing.T) {
	strs := []string{
		`[general]`,
		`k1=v1`,
		`k2 = v2`,
		` k3 = v3 `,
		`k4=" quoted spaces "`,
		`k5 = " quoted spaces " `,
		`k6 = with\nnewline`,
		`k7 = "with\nnewline"`,
		`k8 = a "quoted" word`,
		`k9 = "a \"quoted\" word"`,
	}
	buf := bytes.NewBufferString(strings.Join(strs, "\n"))
	cfg := ini.Parse(buf)

	correct := map[string]string{
		"k1": "v1",
		"k2": "v2",
		"k3": "v3",
		"k4": " quoted spaces ",
		"k5": " quoted spaces ",
		"k6": "with\nnewline",
		"k7": "with\nnewline",
		"k8": "a \"quoted\" word",
		"k9": "a \"quoted\" word",
	}

	for k, v := range correct {
		if v2 := cfg.Get("general", k); v2 != v {
			t.Errorf("Incorrect general.%s, %q != %q", k, v2, v)
		}
	}

	if v := cfg.Get("general", "nonexistant"); v != "" {
		t.Errorf("Unexpected non-empty value %q", v)
	}
}

func TestParseComments(t *testing.T) {
	strs := []string{
		";file comment 1",   // No leading space
		"; file comment 2 ", // Trailing space
		";  file comment 3", // Multiple leading spaces
		"[general]",
		"; b general comment 1", // Comments in unsorted order
		"somekey = somevalue",
		"; a general comment 2",
		"[other]",
		"; other comment 1", // Comments in section with no values
		"; other comment 2",
		"[other2]",
		"; other2 comment 1",
		"; other2 comment 2", // Comments on last section
		"somekey = somevalue",
	}
	buf := bytes.NewBufferString(strings.Join(strs, "\n"))

	correct := map[string][]string{
		"":        []string{"file comment 1", "file comment 2", "file comment 3"},
		"general": []string{"b general comment 1", "a general comment 2"},
		"other":   []string{"other comment 1", "other comment 2"},
		"other2":  []string{"other2 comment 1", "other2 comment 2"},
	}

	cfg := ini.Parse(buf)

	for section, comments := range correct {
		cmts := cfg.Comments(section)
		if len(cmts) != len(comments) {
			t.Errorf("Incorrect number of comments for section %q: %d != %d", section, len(cmts), len(comments))
		} else {
			for i := range comments {
				if cmts[i] != comments[i] {
					t.Errorf("Incorrect comment: %q != %q", cmts[i], comments[i])
				}
			}
		}
	}
}

func TestWrite(t *testing.T) {
	cfg := ini.Config{}
	cfg.Set("general", "k1", "v1")
	cfg.Set("general", "k2", "foo bar")
	cfg.Set("general", "k3", " foo bar ")
	cfg.Set("general", "k4", "foo\nbar")

	var out bytes.Buffer
	cfg.Write(&out)

	correct := `[general]
k1=v1
k2=foo bar
k3=" foo bar "
k4="foo\nbar"

`
	if s := out.String(); s != correct {
		t.Errorf("Incorrect written .INI:\n%s\ncorrect:\n%s", s, correct)
	}
}

func TestSet(t *testing.T) {
	buf := bytes.NewBufferString("[general]\nfoo=bar\nfoo2=bar2\n")
	cfg := ini.Parse(buf)

	cfg.Set("general", "foo", "baz")  // Overwrite existing
	cfg.Set("general", "baz", "quux") // Create new value
	cfg.Set("other", "baz2", "quux2") // Create new section + value

	var out bytes.Buffer
	cfg.Write(&out)

	correct := `[general]
foo=baz
foo2=bar2
baz=quux

[other]
baz2=quux2

`

	if s := out.String(); s != correct {
		t.Errorf("Incorrect INI after set:\n%s", s)
	}
}

func TestSetManyEquals(t *testing.T) {
	buf := bytes.NewBufferString("[general]\nfoo=bar==\nfoo2=bar2==\n")
	cfg := ini.Parse(buf)

	cfg.Set("general", "foo", "baz==")

	var out bytes.Buffer
	cfg.Write(&out)

	correct := `[general]
foo=baz==
foo2=bar2==

`

	if s := out.String(); s != correct {
		t.Errorf("Incorrect INI after set:\n%s", s)
	}
}

func TestRewriteDuplicate(t *testing.T) {
	buf := bytes.NewBufferString("[general]\nfoo=bar==\nfoo=bar2==\n")
	cfg := ini.Parse(buf)

	if v := cfg.Get("general", "foo"); v != "bar2==" {
		t.Errorf("incorrect get %q", v)
	}

	var out bytes.Buffer
	cfg.Write(&out)

	correct := `[general]
foo=bar2==

`

	if s := out.String(); s != correct {
		t.Errorf("Incorrect INI after set:\n%s", s)
	}
}
