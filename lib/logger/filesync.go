// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// TODO:
// 1) Put proper checkbox in the config to enable/disable this feature
// 2) Figure out location of logfile for all OS's (right now it is stored in same location as executable)
// 3) Rotate log files
//

package logger

import (
	"fmt"
	"runtime"
	"os"
	"strings"
	"time"
)

// Global vars used in multiple functions
var filesyncBuffer string
var updaterClient string
var clientFound bool

// The first client to send an index update to the others cronologically seems to always
// be the one that has the original file, which makes sense logically so I'm going with it.
func FindUpdaterClient(deviceID string) {
	if !clientFound {
		updaterClient = deviceID
		clientFound = true
	}
}

// This function just resets the global vars so it searches again next time and only picks
// the first client to send an index update (which seems to always be the one that really
// originated the file/folder changes).
func ResetUpdaterClient () {
	filesyncBuffer = ""
	updaterClient = ""
	clientFound = false
}

// This function records all the client index updates sent to the client
// to a seperate log file than the debug log.
func WriteClientSyncToLog(deviceID string, name string, numFiles int) {
	f, eol := CreateLogFile("FileSync.log")
	defer f.Close()
	
	// If the buffer's empty it's because the change was local, else we do something different (maybe in future)
	if strings.Count(filesyncBuffer, eol) != 0 {
		f.WriteString(updaterClient + eol)
		f.WriteString(fmt.Sprintf("%s [%d object change(s) recieved]", name, numFiles) + eol)
		f.WriteString("-------------------------------------------------------------------------------------" + eol)
		
		// Now write the buffer and clear it for the next time
		f.WriteString(filesyncBuffer + eol)
		filesyncBuffer = ""
	}
}

// This function records all the write file actions sent to the client
// (delete / update) to a seperate log file than the debug log.
func WriteFileSyncToLog(dataIf interface{}) {
	f, eol := CreateLogFile("FileSync.log")
	defer f.Close()
	
	// Don't write this right away but add it to the buffer to be written after the client info
	data := dataIf.(map[string]interface{})
	filesyncBuffer += time.Now().Format(time.StampMilli) + fmt.Sprintf(": [ %v ] %v %v \"%s\"", data["folder"], data["action"], data["type"], data["item"]) + eol
	
	f.Sync()
}

// Create the file if it doesn't exist and add disclaimer, otherwise just open it
// and append to the end.  Also fixup the eol character for whichever OS we're on.
func CreateLogFile(filename string) (*os.File, string) {
	var f *os.File
	
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		f, _ = os.Create(filename)
		f.WriteString(LogInit())
	} else {
		f, _ = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0666)
	}
	
	eol := "\n"
	if runtime.GOOS == "windows" {
		eol = "\r\n"
	}
	
	return f, eol
}

func LogInit() string {
	initText := `LOG DISCLAIMER:
Please understand this is a log of only what updates this computer recieves and
from which other clients.  THIS LOG WILL NOT TELL YOU WHAT CHANGES ORIGINATED
OUTBOUND FROM THIS COMPUTER!  Just what was propagated inbound to this computer
by other computers and when.  That way if a file was deleted inadvertently a
user could at least trace it back to the original computer by looking at these
logs and any other nodes logs for other changes to know where the deletion came
from.
Also keep in mind this functionality assumes ALL your computers are exactly in
time sync with each other.  If they are not, the timestamps in this log (using
your local time zone not UTC) will be much less helpful at best and completely
misleading at worse.  You are highly advised to use a time server on your
network for all your nodes.
--------------------------------------------------------------------------------


`
	if runtime.GOOS == "windows" {
		initText = strings.Replace(initText, "\n", "\r\n", -1)
	}
	
	return initText
}
