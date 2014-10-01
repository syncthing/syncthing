// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package luhn_test

import (
	"testing"

	"github.com/syncthing/syncthing/internal/luhn"
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
