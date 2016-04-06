// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
)

type localBlockPuller struct {
	model       *Model
	folders     []string
	folderRoots map[string]string
}

func (p *localBlockPuller) Request(file string, offset int64, hash []byte, buf []byte) error {
	fn := func(folder, file string, index int32) bool {
		fd, err := os.Open(filepath.Join(p.folderRoots[folder], file))
		if err != nil {
			return false
		}

		_, err = fd.ReadAt(buf, protocol.BlockSize*int64(index))
		fd.Close()
		if err != nil {
			return false
		}

		actualHash, err := scanner.VerifyBuffer(buf, protocol.BlockInfo{
			Size: int32(len(buf)),
			Hash: hash,
		})
		if err != nil {
			if hash != nil {
				l.Debugf("Finder block mismatch in %s:%s:%d expected %q got %q", folder, file, index, hash, actualHash)
				err = p.model.finder.Fix(folder, file, index, hash, actualHash)
				if err != nil {
					l.Warnln("finder fix:", err)
				}
			} else {
				l.Debugln("Finder failed to verify buffer", err)
			}
			return false
		}
		return true
	}

	if p.model.finder.Iterate(p.folders, hash, fn) {
		return nil
	}

	return errors.New("no such block")
}

type networkBlockPuller struct {
	model  *Model
	folder string
}

func (p *networkBlockPuller) Request(file string, offset int64, hash []byte, buf []byte) error {
	potentialDevices := p.model.Availability(p.folder, file)
	for {
		// Select the least busy device to pull the block from. If we found no
		// feasible device at all, fail the block (and in the long run, the
		// file).
		selected := activity.leastBusy(potentialDevices)
		if selected == (protocol.DeviceID{}) {
			l.Debugln("request:", p.folder, file, offset, len(buf), errNoDevice)
			return errNoDevice
		}

		potentialDevices = removeDevice(potentialDevices, selected)

		// Fetch the block, while marking the selected device as in use so that
		// leastBusy can select another device when someone else asks.
		activity.using(selected)
		tmpBbuf, err := p.model.requestGlobal(selected, p.folder, file, offset, len(buf), hash, 0, nil)
		activity.done(selected)
		if err != nil {
			l.Debugln("request:", p.folder, file, offset, len(buf), "returned error:", err)
			return err
		}

		// Verify that the received block matches the desired hash, if not
		// try pulling it from another device.
		_, err = scanner.VerifyBuffer(tmpBbuf, protocol.BlockInfo{
			Size: int32(len(buf)),
			Hash: hash,
		})
		if err != nil {
			l.Debugln("request:", p.folder, file, offset, len(buf), err)
			continue
		}

		l.Debugln("completed request:", p.folder, file, offset, len(buf))
		copy(buf, tmpBbuf)
		break
	}

	return nil
}
