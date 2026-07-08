// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cmdutil

import "testing"

var keywords = map[string]string{
	"AFFIXLESS_KEYWORD":   "bare",
	"%DOS_STYLE_KEYWORD%": "dos",
}

func TestFormattedCommandSuccessRealKeywords(t *testing.T) {
	cmd, err := FormattedCommand("echo AFFIXLESS_KEYWORD %DOS_STYLE_KEYWORD%", keywords)
	if err != nil {
		t.Fatal(err)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	const expectedOutput = "bare dos\n"

	if string(output) != expectedOutput {
		t.Errorf("expected %s as command output, got %s", expectedOutput, string(output))
	}
}

func TestFormattedCommandSuccessNilKeywords(t *testing.T) {
	const testText = "this command should be executed verbatim"
	cmd, err := FormattedCommand("echo "+testText, nil)
	if err != nil {
		t.Fatal(err)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	const expectedOutput = testText + "\n"
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

func TestUnsafeFormattedCommand(t *testing.T) {
	context := map[string]string{
		"%FOLDER_PATH%":       "/home/testuser/Sync",
		"%FILE_PATH%":         "TestUnsafeFormattedCommand",
	}

	cases := []struct {
		cmd  string
		safe bool
	}{
		{`echo %FOLDER_PATH% %FILE_PATH%`, true},
		{`echo "%FOLDER_PATH% %FILE_PATH%"`, false},
		{`echo %FOLDER_PATH%/%FILE_PATH%`, true},
		{`echo "%FOLDER_PATH%/%FILE_PATH%"`, true},
		{`echo '%FOLDER_PATH%/%FILE_PATH%'`, true},
		{`echo "'%FOLDER_PATH%/%FILE_PATH%'"`, false},
		{`sh -c "echo '%FOLDER_PATH%/%FILE_PATH%'"`, false},
		{`sh -c "echo %FOLDER_PATH%/%FILE_PATH%"`, false},
	}

	for _, tc := range cases {
		res, err := FormattedCommand(tc.cmd, context)
		if tc.safe && err != nil {
			t.Fatal(err)
		}
		if !tc.safe && err == nil {
			t.Logf("%q", res.Path)
			t.Logf("%q", res.Args)
			t.Errorf("should be unsafe: %q", tc.cmd)
		}
	}
}
