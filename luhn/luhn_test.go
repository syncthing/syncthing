package luhn_test

import (
	"testing"

	"github.com/calmh/syncthing/luhn"
)

func TestGenerate(t *testing.T) {
	a := luhn.Alphabet("abcdef")
	c := a.Generate("abcdef")
	if c != 'e' {
		t.Errorf("Incorrect check digit %c != e", c)
	}
}

func TestValidate(t *testing.T) {
	a := luhn.Alphabet("abcdef")
	if !a.Validate("abcdefe") {
		t.Errorf("Incorrect validation response for abcdefe")
	}
	if a.Validate("abcdefd") {
		t.Errorf("Incorrect validation response for abcdefd")
	}
}
