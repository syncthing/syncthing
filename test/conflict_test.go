// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package integration

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/rc"
)

func TestConflictsDefault(t *testing.T) {
	log.Println("Generating files...")

	// Create a source folder with some data in it.
	srcDir := generateTree(t, 1) // @TODO change to 100
	// Create an empty destination folder to hold the synced data.
	dstDir := t.TempDir()

	srcTestfile := filepath.Join(srcDir, "testfile.txt")
	dstTestfile := filepath.Join(srcDir, "testfile.txt")
	os.WriteFile(srcTestfile, []byte("hello\n"), 0o664)

	ctx := context.Background()
	if dl, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, dl)
		defer cancel()
	}

	// Spin up two instances to sync the data.
	src, dst, folderID := testSyncTwoDevicesFolders(ctx, t, srcDir, dstDir)

	// Check that the destination folder now contains the same files as the source folder.
	compareTrees(t, srcDir, dstDir)

	log.Println("Introducing a conflict (simultaneous edit)...")

	srcAPI := rc.NewAPI(src.apiAddress, src.apiKey)
	dstAPI := rc.NewAPI(dst.apiAddress, dst.apiKey)

	err := dstAPI.Post("/rest/system/pause?device="+dst.deviceID.String(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	fd, err := os.OpenFile(srcTestfile, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("text added to src\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	fd, err = os.OpenFile(dstTestfile, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("different text added to dst to change the size\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = dstAPI.Post("/rest/system/resume?device="+dst.deviceID.String(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Syncing...")

	delaySeconds := 86400

	err = srcAPI.Post(fmt.Sprintf("/rest/db/scan?folder=%s&next=%d", url.QueryEscape(folderID), delaySeconds), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = dstAPI.Post(fmt.Sprintf("/rest/db/scan?folder=%s&next=%d", url.QueryEscape(folderID), delaySeconds), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var srcDur, dstDur map[string]time.Duration
	var srcErr, dstErr error

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		srcDur, srcErr = waitForSync(ctx, folderID, srcAPI)
	}()
	go func() {
		defer wg.Done()
		dstDur, dstErr = waitForSync(ctx, folderID, dstAPI)
	}()
	wg.Wait()

	if srcErr != nil && !errors.Is(srcErr, context.DeadlineExceeded) {
		t.Error(srcErr)
	}

	if dstErr != nil && !errors.Is(dstErr, context.DeadlineExceeded) {
		t.Error(dstErr)
	}

	t.Log("src durations:", srcDur)
	t.Log("dst durations:", dstDur)

	// Expect one conflict file, created on either side.

	srcPattern := filepath.Join(srcDir, "*sync-conflict*")
	srcFiles, err := filepath.Glob(srcPattern)
	if err != nil {
		t.Fatal(err)
	}
	dstPattern := filepath.Join(dstDir, "*sync-conflict*")
	dstFiles, err := filepath.Glob(dstPattern)
	if err != nil {
		t.Fatal(err)
	}
	files := append(srcFiles, dstFiles...)

	if len(files) != 2 {
		t.Errorf("Expected 1 conflicted file on each side, instead of totally %d", len(files))
	} else if filepath.Base(files[0]) != filepath.Base(files[1]) {
		t.Errorf(`Expected same conflicted file on both sides, got "%v" and "%v"`, files[0], files[1])
	}

	// log.Println("Introducing a conflict (edit plus delete)...")

	// if err := sender.PauseDevice(receiver.ID()); err != nil {
	// 	t.Fatal(err)
	// }

	// err = os.Remove("s1/testfile.txt")
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// fd, err = os.OpenFile("s2/testfile.txt", os.O_WRONLY|os.O_APPEND, 0644)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// _, err = fd.WriteString("more text added to s2\n")
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// err = fd.Close()
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// if err := sender.ResumeDevice(receiver.ID()); err != nil {
	// 	t.Fatal(err)
	// }

	// log.Println("Syncing...")

	// if err := sender.RescanDelay("default", 86400); err != nil {
	// 	t.Fatal(err)
	// }
	// if err := receiver.RescanDelay("default", 86400); err != nil {
	// 	t.Fatal(err)
	// }
	// rc.AwaitSync("default", sender, receiver)

	// // The conflict is resolved to the advantage of the edit over the delete.
	// // As such, we get the edited content synced back to s1 where it was
	// // removed.

	// files, err = filepath.Glob("s2/*sync-conflict*")
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// if len(files) != 1 {
	// 	t.Errorf("Expected 1 conflicted files instead of %d", len(files))
	// }
	// bs, err := os.ReadFile("s1/testfile.txt")
	// if err != nil {
	// 	t.Error("reading file:", err)
	// }
	// if !bytes.Contains(bs, []byte("more text added to s2")) {
	// 	t.Error("s1/testfile.txt should contain data added in s2")
	// }
}

func TestConflictsInitialMerge(t *testing.T) {
	// @TODO
}

func TestConflictsIndexReset(t *testing.T) {
	// @TODO
}

func TestConflictsSameContent(t *testing.T) {
	// @TODO
}
