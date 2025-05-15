// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// cmdutil implements utilities for running external commands
package cmdutil

import "testing"

var keywords = map[string]string{
	"nggyu":               "bait", // bare keyword with no prefix or suffix
	"%DOS_STYLE_KEYWORD%": "dos",
	"$UNIX_STYLE_KEYWORD": "unix",
}

func TestFormattedCommandSuccessRealKeywords(t *testing.T) {
	cmd, err := FormattedCommand("echo nggyu %DOS_STYLE_KEYWORD% $UNIX_STYLE_KEYWORD", keywords)
	if err != nil {
		t.Fatal(err)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	const expectedOutput = "bait dos unix\n"

	if string(output) != expectedOutput {
		t.Errorf("expected %s as command output, got %s", expectedOutput, string(output))
	}
}

func TestFormattedCommandSuccessNilKeywords(t *testing.T) {
	cmd, err := FormattedCommand("echo this command should be executed verbatim", nil)
	if err != nil {
		t.Fatal(err)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	const expectedOutput = "this command should be executed verbatim\n"

	if string(output) != expectedOutput {
		t.Errorf("expected %s as command output, got %s", expectedOutput, string(output))
	}
}

func TestFormattedCommandFailBlankCommand(t *testing.T) {
	_, err := FormattedCommand("", keywords)
	if err == nil {
		t.Error("blank commands should fail")
	}
}

func TestFormattedCommandFailBlankCommandNilKeywords(t *testing.T) {
	_, err := FormattedCommand("", nil)
	if err == nil {
		t.Error("blank commands should fail even if keywords are nil")
	}
}
