// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"testing"
)

func TestGenerate(t *testing.T) {
	// Base 6 Luhn
	a := luhnAlphabet("abcdef")
	c, err := a.generate("abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if c != 'e' {
		t.Errorf("Incorrect check digit %c != e", c)
	}

	// Base 10 Luhn
	a = luhnAlphabet("0123456789")
	c, err = a.generate("7992739871")
	if err != nil {
		t.Fatal(err)
	}
	if c != '3' {
		t.Errorf("Incorrect check digit %c != 3", c)
	}
}

func TestInvalidString(t *testing.T) {
	a := luhnAlphabet("ABC")
	_, err := a.generate("7992739871")
	t.Log(err)
	if err == nil {
		t.Error("Unexpected nil error")
	}
}

func TestValidate(t *testing.T) {
	a := luhnAlphabet("abcdef")
	if !a.luhnValidate("abcdefe") {
		t.Errorf("Incorrect validation response for abcdefe")
	}
	if a.luhnValidate("abcdefd") {
		t.Errorf("Incorrect validation response for abcdefd")
	}
}
