// Copyright (C) 2014 Jakob Borg

package luhn_test

import (
	"testing"

	"github.com/calmh/luhn"
)

func TestGenerate(t *testing.T) {
	// Base 6 Luhn
	a := luhn.Alphabet("abcdef")
	c, err := a.Generate("abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if c != 'e' {
		t.Errorf("Incorrect check digit %c != e", c)
	}

	// Base 10 Luhn
	a = luhn.Alphabet("0123456789")
	c, err = a.Generate("7992739871")
	if err != nil {
		t.Fatal(err)
	}
	if c != '3' {
		t.Errorf("Incorrect check digit %c != 3", c)
	}
}

func TestInvalidString(t *testing.T) {
	a := luhn.Alphabet("ABC")
	_, err := a.Generate("7992739871")
	t.Log(err)
	if err == nil {
		t.Error("Unexpected nil error")
	}
}

func TestBadAlphabet(t *testing.T) {
	a := luhn.Alphabet("01234566789")
	_, err := a.Generate("7992739871")
	t.Log(err)
	if err == nil {
		t.Error("Unexpected nil error")
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
