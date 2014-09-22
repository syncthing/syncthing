// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

/*
__        __               _             _
\ \      / /_ _ _ __ _ __ (_)_ __   __ _| |
 \ \ /\ / / _` | '__| '_ \| | '_ \ / _` | |
  \ V  V / (_| | |  | | | | | | | | (_| |_|
   \_/\_/ \__,_|_|  |_| |_|_|_| |_|\__, (_)
                                   |___/

The code in this file is a piece of crap. Don't base anything on it.
Refactorin ongoing in new-puller branch.

__        __               _             _
\ \      / /_ _ _ __ _ __ (_)_ __   __ _| |
 \ \ /\ / / _` | '__| '_ \| | '_ \ / _` | |
  \ V  V / (_| | |  | | | | | | | | (_| |_|
   \_/\_/ \__,_|_|  |_| |_|_|_| |_|\__, (_)
                                   |___/
*/

package model

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/events"
	"github.com/syncthing/syncthing/internal/osutil"
	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syncthing/syncthing/internal/scanner"
	"github.com/syncthing/syncthing/internal/versioner"
)

type requestResult struct {
	node     protocol.NodeID
	file     protocol.FileInfo
	filepath string // full filepath name
	offset   int64
	data     []byte
	err      error
}

type openFile struct {
	filepath     string // full filepath name
	temp         string // temporary filename
	availability []protocol.NodeID
	file         *os.File
	err          error // error when opening or writing to file, all following operations are cancelled
	outstanding  int   // number of requests we still have outstanding
	done         bool  // we have sent all requests for this file
}

type activityMap map[protocol.NodeID]int

// Queue about this many blocks each puller iteration. More blocks means
// longer iterations and better efficiency; fewer blocks reduce memory
// consumption. 1000 blocks ~= 1000 * 128 KiB ~= 125 MiB of data.
const pullIterationBlocks = 1000

func (m activityMap) leastBusyNode(availability []protocol.NodeID, isValid func(protocol.NodeID) bool) protocol.NodeID {
	var low int = 2<<30 - 1
	var selected protocol.NodeID
	for _, node := range availability {
		usage := m[node]
		if usage < low && isValid(node) {
			low = usage
			selected = node
		}
	}
	m[selected]++
	return selected
}

func (m activityMap) decrease(node protocol.NodeID) {
	m[node]--
}

var errNoNode = errors.New("no available source node")

type puller struct {
	cfg               *config.Configuration
	repoCfg           config.RepositoryConfiguration
	bq                blockQueue
	slots             int
	model             *Model
	oustandingPerNode activityMap
	openFiles         map[string]openFile
	requestSlots      chan bool
	blocks            chan bqBlock
	requestResults    chan requestResult
	versioner         versioner.Versioner
	errors            int
}

func newPuller(repoCfg config.RepositoryConfiguration, model *Model, slots int, cfg *config.Configuration) *puller {
	p := &puller{
		cfg:               cfg,
		repoCfg:           repoCfg,
		slots:             slots,
		model:             model,
		oustandingPerNode: make(activityMap),
		openFiles:         make(map[string]openFile),
		requestSlots:      make(chan bool, slots),
		blocks:            make(chan bqBlock),
		requestResults:    make(chan requestResult),
	}

	if len(repoCfg.Versioning.Type) > 0 {
		factory, ok := versioner.Factories[repoCfg.Versioning.Type]
		if !ok {
			l.Fatalf("Requested versioning type %q that does not exist", repoCfg.Versioning.Type)
		}
		p.versioner = factory(repoCfg.ID, repoCfg.Directory, repoCfg.Versioning.Params)
	}

	if slots > 0 {
		// Read/write
		if debug {
			l.Debugf("starting puller; repo %q dir %q slots %d", repoCfg.ID, repoCfg.Directory, slots)
		}
		go p.run()
	} else {
		// Read only
		if debug {
			l.Debugf("starting puller; repo %q dir %q (read only)", repoCfg.ID, repoCfg.Directory)
		}
		go p.runRO()
	}
	return p
}

func (p *puller) run() {
	changed := true
	scanintv := time.Duration(p.repoCfg.RescanIntervalS) * time.Second
	lastscan := time.Now()
	var prevVer uint64
	var queued int

	// Load up the request slots
	for i := 0; i < cap(p.requestSlots); i++ {
		p.requestSlots <- true
	}

	for {
		if sc, sl := cap(p.requestSlots), len(p.requestSlots); sl != sc {
			panic(fmt.Sprintf("Incorrect number of slots; %d != %d", sl, sc))
		}

		// Run the pulling loop as long as there are blocks to fetch
		prevVer, queued = p.queueNeededBlocks(prevVer)
		if queued > 0 {
			p.errors = 0

		pull:
			for {
				select {
				case res := <-p.requestResults:
					p.model.setState(p.repoCfg.ID, RepoSyncing)
					changed = true
					p.requestSlots <- true
					p.handleRequestResult(res)

				case <-p.requestSlots:
					b, ok := p.bq.get()

					if !ok {
						if debug {
							l.Debugf("%q: pulling loop needs more blocks", p.repoCfg.ID)
						}

						if p.errors > 0 && p.errors >= queued {
							p.requestSlots <- true
							break pull
						}

						prevVer, _ = p.queueNeededBlocks(prevVer)
						b, ok = p.bq.get()
					}

					if !ok && len(p.openFiles) == 0 {
						// Nothing queued, nothing outstanding
						if debug {
							l.Debugf("%q: pulling loop done", p.repoCfg.ID)
						}
						p.requestSlots <- true
						break pull
					}

					if !ok {
						// Nothing queued, but there are still open files.
						// Give the situation a moment to change.
						if debug {
							l.Debugf("%q: pulling loop paused", p.repoCfg.ID)
						}
						p.requestSlots <- true
						time.Sleep(100 * time.Millisecond)
						continue pull
					}

					if debug {
						l.Debugf("queueing %q / %q offset %d copy %d", p.repoCfg.ID, b.file.Name, b.block.Offset, len(b.copy))
					}
					p.model.setState(p.repoCfg.ID, RepoSyncing)
					changed = true
					if p.handleBlock(b) {
						// Block was fully handled, free up the slot
						p.requestSlots <- true
					}
				}
			}

			if p.errors > 0 && p.errors >= queued {
				l.Warnf("All remaining files failed to sync. Stopping repo %q.", p.repoCfg.ID)
				invalidateRepo(p.cfg, p.repoCfg.ID, errors.New("too many errors, check logs"))
				return
			}
		}

		if changed {
			p.model.setState(p.repoCfg.ID, RepoCleaning)
			p.clean()
			changed = false
		}

		p.model.setState(p.repoCfg.ID, RepoIdle)

		// Do a rescan if it's time for it
		if time.Since(lastscan) > scanintv {
			if debug {
				l.Debugf("%q: time for rescan", p.repoCfg.ID)
			}

			err := p.model.ScanRepo(p.repoCfg.ID)
			if err != nil {
				invalidateRepo(p.cfg, p.repoCfg.ID, err)
				return
			}
			lastscan = time.Now()
		}

		time.Sleep(5 * time.Second)
	}
}

func (p *puller) runRO() {
	walkTicker := time.Tick(time.Duration(p.repoCfg.RescanIntervalS) * time.Second)

	for _ = range walkTicker {
		if debug {
			l.Debugf("%q: time for rescan", p.repoCfg.ID)
		}
		err := p.model.ScanRepo(p.repoCfg.ID)
		if err != nil {
			invalidateRepo(p.cfg, p.repoCfg.ID, err)
			return
		}
	}
}

// clean deletes orphaned temporary files and directories that should no
// longer exist.
func (p *puller) clean() {
	var deleteDirs []string
	var changed = 0

	var walkFn = func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Mode().IsRegular() && defTempNamer.IsTemporary(path) {
			os.Remove(path)
		}

		if !info.IsDir() {
			return nil
		}

		rn, err := filepath.Rel(p.repoCfg.Directory, path)
		if err != nil {
			return nil
		}

		if rn == "." {
			return nil
		}

		if filepath.Base(rn) == ".stversions" {
			return filepath.SkipDir
		}

		cur := p.model.CurrentRepoFile(p.repoCfg.ID, rn)
		if cur.Name != rn {
			// No matching dir in current list; weird
			if debug {
				l.Debugf("missing dir: %s; %v", rn, cur)
			}
			return nil
		}

		if protocol.IsDeleted(cur.Flags) {
			if debug {
				l.Debugf("queue delete dir: %v", cur)
			}

			// We queue the directories to delete since we walk the
			// tree in depth first order and need to remove the
			// directories in the opposite order.

			deleteDirs = append(deleteDirs, path)
			return nil
		}

		if !p.repoCfg.IgnorePerms && protocol.HasPermissionBits(cur.Flags) && !scanner.PermsEqual(cur.Flags, uint32(info.Mode())) {
			err := os.Chmod(path, os.FileMode(cur.Flags)&os.ModePerm)
			if err != nil {
				l.Warnf("Restoring folder flags: %q: %v", path, err)
			} else {
				changed++
				if debug {
					l.Debugf("restored dir flags: %o -> %v", info.Mode()&os.ModePerm, cur)
				}
			}
		}

		return nil
	}

	for {
		deleteDirs = nil
		changed = 0
		filepath.Walk(p.repoCfg.Directory, walkFn)

		var deleted = 0
		// Delete any queued directories
		for i := len(deleteDirs) - 1; i >= 0; i-- {
			dir := deleteDirs[i]
			if debug {
				l.Debugln("delete dir:", dir)
			}
			err := os.Remove(dir)
			if err == nil {
				deleted++
			} else {
				l.Warnln("Delete dir:", err)
			}
		}

		if debug {
			l.Debugf("changed %d, deleted %d dirs", changed, deleted)
		}

		if changed+deleted == 0 {
			return
		}
	}
}

func (p *puller) handleRequestResult(res requestResult) {
	p.oustandingPerNode.decrease(res.node)
	f := res.file

	of, ok := p.openFiles[f.Name]
	if !ok {
		// no entry in openFiles means there was an error and we've cancelled the operation
		return
	}

	if res.err != nil {
		// This request resulted in an error
		of.err = res.err
		if debug {
			l.Debugf("pull: not writing %q / %q offset %d: %v; (done=%v, outstanding=%d)", p.repoCfg.ID, f.Name, res.offset, res.err, of.done, of.outstanding)
		}
	} else if of.err == nil {
		// This request was sucessfull and nothing has failed previously either
		_, of.err = of.file.WriteAt(res.data, res.offset)
		if debug {
			l.Debugf("pull: wrote %q / %q offset %d len %d outstanding %d done %v", p.repoCfg.ID, f.Name, res.offset, len(res.data), of.outstanding, of.done)
		}
	}

	of.outstanding--
	p.openFiles[f.Name] = of

	if of.done && of.outstanding == 0 {
		p.closeFile(f)
	}
}

// handleBlock fulfills the block request by copying, ignoring or fetching
// from the network. Returns true if the block was fully handled
// synchronously, i.e. if the slot can be reused.
func (p *puller) handleBlock(b bqBlock) bool {
	f := b.file

	// For directories, making sure they exist is enough.
	// Deleted directories we mark as handled and delete later.
	if protocol.IsDirectory(f.Flags) {
		if !protocol.IsDeleted(f.Flags) {
			path := filepath.Join(p.repoCfg.Directory, f.Name)
			_, err := os.Stat(path)
			if err != nil && os.IsNotExist(err) {
				if debug {
					l.Debugf("create dir: %v", f)
				}
				err = os.MkdirAll(path, os.FileMode(f.Flags&0777))
				if err != nil {
					p.errors++
					l.Infof("mkdir: error: %q: %v", path, err)
				}
			}
		} else if debug {
			l.Debugf("ignore delete dir: %v", f)
		}
		p.model.updateLocal(p.repoCfg.ID, f)
		return true
	}

	if len(b.copy) > 0 && len(b.copy) == len(b.file.Blocks) && b.last {
		// We are supposed to copy the entire file, and then fetch nothing.
		// We don't actually need to make the copy.
		if debug {
			l.Debugln("taking shortcut:", f)
		}
		fp := filepath.Join(p.repoCfg.Directory, f.Name)
		t := time.Unix(f.Modified, 0)
		err := os.Chtimes(fp, t, t)
		if err != nil {
			l.Infof("chtimes: error: %q / %q: %v", p.repoCfg.ID, f.Name, err)
		}
		if !p.repoCfg.IgnorePerms && protocol.HasPermissionBits(f.Flags) {
			err = os.Chmod(fp, os.FileMode(f.Flags&0777))
			if err != nil {
				l.Infof("chmod: error: %q / %q: %v", p.repoCfg.ID, f.Name, err)
			}
		}

		events.Default.Log(events.ItemStarted, map[string]string{
			"repo": p.repoCfg.ID,
			"item": f.Name,
		})

		p.model.updateLocal(p.repoCfg.ID, f)
		return true
	}

	of, ok := p.openFiles[f.Name]
	of.done = b.last

	if !ok {
		if debug {
			l.Debugf("pull: %q: opening file %q", p.repoCfg.ID, f.Name)
		}

		events.Default.Log(events.ItemStarted, map[string]string{
			"repo": p.repoCfg.ID,
			"item": f.Name,
		})

		of.availability = p.model.repoFiles[p.repoCfg.ID].Availability(f.Name)
		of.filepath = filepath.Join(p.repoCfg.Directory, f.Name)
		of.temp = filepath.Join(p.repoCfg.Directory, defTempNamer.TempName(f.Name))

		dirName := filepath.Dir(of.filepath)
		info, err := os.Stat(dirName)
		if err != nil {
			err = os.MkdirAll(dirName, 0777)
			if debug && err != nil {
				l.Debugf("mkdir: error: %q / %q: %v", p.repoCfg.ID, f.Name, err)
			}
		} else {
			// We need to make sure the directory is writeable so we can create files in it
			if dirName != p.repoCfg.Directory {
				err = os.Chmod(dirName, 0777)
				if debug && err != nil {
					l.Debugf("make writeable: error: %q / %q: %v", p.repoCfg.ID, f.Name, err)
				}
			}
			// Change it back after creating the file, to minimize the time window with incorrect permissions
			defer os.Chmod(dirName, info.Mode())
		}

		of.file, of.err = os.Create(of.temp)
		if of.err != nil {
			p.errors++
			l.Infof("create: error: %q / %q: %v", p.repoCfg.ID, f.Name, of.err)
			if !b.last {
				p.openFiles[f.Name] = of
			}
			return true
		}
		osutil.HideFile(of.temp)
	}

	if of.err != nil {
		// We have already failed this file.
		if debug {
			l.Debugf("pull: error: %q / %q has already failed: %v", p.repoCfg.ID, f.Name, of.err)
		}
		if b.last {
			delete(p.openFiles, f.Name)
		}

		return true
	}

	p.openFiles[f.Name] = of

	switch {
	case len(b.copy) > 0:
		p.handleCopyBlock(b)
		return true

	case b.block.Size > 0:
		return p.handleRequestBlock(b)

	default:
		p.handleEmptyBlock(b)
		return true
	}
}

func (p *puller) handleCopyBlock(b bqBlock) {
	// We have blocks to copy from the existing file
	f := b.file
	of := p.openFiles[f.Name]

	if debug {
		l.Debugf("pull: copying %d blocks for %q / %q", len(b.copy), p.repoCfg.ID, f.Name)
	}

	var exfd *os.File
	exfd, of.err = os.Open(of.filepath)
	if of.err != nil {
		p.errors++
		l.Infof("open: error: %q / %q: %v", p.repoCfg.ID, f.Name, of.err)
		of.file.Close()
		of.file = nil

		p.openFiles[f.Name] = of
		return
	}
	defer exfd.Close()

	for _, b := range b.copy {
		bs := make([]byte, b.Size)
		_, of.err = exfd.ReadAt(bs, b.Offset)
		if of.err == nil {
			_, of.err = of.file.WriteAt(bs, b.Offset)
		}
		if of.err != nil {
			p.errors++
			l.Infof("write: error: %q / %q: %v", p.repoCfg.ID, f.Name, of.err)
			exfd.Close()
			of.file.Close()
			of.file = nil

			p.openFiles[f.Name] = of
			return
		}
	}
}

// handleRequestBlock tries to pull a block from the network. Returns true if
// the block could _not_ be fetched (i.e. it was fully handled, matching the
// return criteria of handleBlock)
func (p *puller) handleRequestBlock(b bqBlock) bool {
	f := b.file
	of, ok := p.openFiles[f.Name]
	if !ok {
		panic("bug: request for non-open file")
	}

	node := p.oustandingPerNode.leastBusyNode(of.availability, p.model.ConnectedTo)
	if node == (protocol.NodeID{}) {
		of.err = errNoNode
		if of.file != nil {
			of.file.Close()
			of.file = nil
			os.Remove(of.temp)
			if debug {
				l.Debugf("pull: no source for %q / %q; closed", p.repoCfg.ID, f.Name)
			}
		}
		if b.last {
			if debug {
				l.Debugf("pull: no source for %q / %q; deleting", p.repoCfg.ID, f.Name)
			}
			delete(p.openFiles, f.Name)
		} else {
			if debug {
				l.Debugf("pull: no source for %q / %q; await more blocks", p.repoCfg.ID, f.Name)
			}
			p.openFiles[f.Name] = of
		}
		return true
	}

	of.outstanding++
	p.openFiles[f.Name] = of

	go func(node protocol.NodeID, b bqBlock) {
		if debug {
			l.Debugf("pull: requesting %q / %q offset %d size %d from %q outstanding %d", p.repoCfg.ID, f.Name, b.block.Offset, b.block.Size, node, of.outstanding)
		}

		bs, err := p.model.requestGlobal(node, p.repoCfg.ID, f.Name, b.block.Offset, int(b.block.Size), nil)
		p.requestResults <- requestResult{
			node:     node,
			file:     f,
			filepath: of.filepath,
			offset:   b.block.Offset,
			data:     bs,
			err:      err,
		}
	}(node, b)

	return false
}

func (p *puller) handleEmptyBlock(b bqBlock) {
	f := b.file
	of := p.openFiles[f.Name]

	if b.last {
		if of.err == nil {
			of.file.Close()
		}
	}

	if protocol.IsDeleted(f.Flags) {
		if debug {
			l.Debugf("pull: delete %q", f.Name)
		}
		os.Remove(of.temp)

		// Ensure the file and the directory it is in is writeable so we can remove the file
		dirName := filepath.Dir(of.filepath)
		err := os.Chmod(of.filepath, 0666)
		if debug && err != nil {
			l.Debugf("make writeable: error: %q: %v", of.filepath, err)
		}
		if dirName != p.repoCfg.Directory {
			info, err := os.Stat(dirName)
			if err != nil {
				l.Debugln("weird! can't happen?", err)
			}
			err = os.Chmod(dirName, 0777)
			if debug && err != nil {
				l.Debugf("make writeable: error: %q: %v", dirName, err)
			}
			// Change it back after deleting the file, to minimize the time window with incorrect permissions
			defer os.Chmod(dirName, info.Mode())
		}
		if p.versioner != nil {
			if debug {
				l.Debugln("pull: deleting with versioner")
			}
			if err := p.versioner.Archive(of.filepath); err == nil {
				p.model.updateLocal(p.repoCfg.ID, f)
			} else if debug {
				l.Debugln("pull: error:", err)
			}
		} else if err := os.Remove(of.filepath); err == nil || os.IsNotExist(err) {
			p.model.updateLocal(p.repoCfg.ID, f)
		}
	} else {
		if debug {
			l.Debugf("pull: no blocks to fetch and nothing to copy for %q / %q", p.repoCfg.ID, f.Name)
		}
		t := time.Unix(f.Modified, 0)
		if os.Chtimes(of.temp, t, t) != nil {
			delete(p.openFiles, f.Name)
			return
		}
		if !p.repoCfg.IgnorePerms && protocol.HasPermissionBits(f.Flags) && os.Chmod(of.temp, os.FileMode(f.Flags&0777)) != nil {
			delete(p.openFiles, f.Name)
			return
		}
		osutil.ShowFile(of.temp)
		if osutil.Rename(of.temp, of.filepath) == nil {
			p.model.updateLocal(p.repoCfg.ID, f)
		}
	}
	delete(p.openFiles, f.Name)
}

func (p *puller) queueNeededBlocks(prevVer uint64) (uint64, int) {
	curVer := p.model.LocalVersion(p.repoCfg.ID)
	if curVer == prevVer {
		return curVer, 0
	}

	if debug {
		l.Debugf("%q: checking for more needed blocks", p.repoCfg.ID)
	}

	queued := 0
	files := make([]protocol.FileInfo, 0, indexBatchSize)
	for _, f := range p.model.NeedFilesRepoLimited(p.repoCfg.ID, indexBatchSize, pullIterationBlocks) {
		if _, ok := p.openFiles[f.Name]; ok {
			continue
		}
		files = append(files, f)
	}

	perm := rand.Perm(len(files))
	for _, idx := range perm {
		f := files[idx]
		lf := p.model.CurrentRepoFile(p.repoCfg.ID, f.Name)
		have, need := scanner.BlockDiff(lf.Blocks, f.Blocks)
		if debug {
			l.Debugf("need:\n  local: %v\n  global: %v\n  haveBlocks: %v\n  needBlocks: %v", lf, f, have, need)
		}
		queued++
		p.bq.put(bqAdd{
			file: f,
			have: have,
			need: need,
		})
	}

	if debug && queued > 0 {
		l.Debugf("%q: queued %d items", p.repoCfg.ID, queued)
	}

	if queued > 0 {
		return prevVer, queued
	} else {
		return curVer, 0
	}
}

func (p *puller) closeFile(f protocol.FileInfo) {
	if debug {
		l.Debugf("pull: closing %q / %q", p.repoCfg.ID, f.Name)
	}

	of := p.openFiles[f.Name]
	err := of.file.Close()
	if err != nil {
		p.errors++
		l.Infof("close: error: %q / %q: %v", p.repoCfg.ID, f.Name, err)
	}
	defer os.Remove(of.temp)

	delete(p.openFiles, f.Name)

	fd, err := os.Open(of.temp)
	if err != nil {
		p.errors++
		l.Infof("open: error: %q / %q: %v", p.repoCfg.ID, f.Name, err)
		return
	}
	hb, _ := scanner.Blocks(fd, scanner.StandardBlockSize, f.Size())
	fd.Close()

	if l0, l1 := len(hb), len(f.Blocks); l0 != l1 {
		if debug {
			l.Debugf("pull: %q / %q: nblocks %d != %d", p.repoCfg.ID, f.Name, l0, l1)
		}
		return
	}

	for i := range hb {
		if bytes.Compare(hb[i].Hash, f.Blocks[i].Hash) != 0 {
			if debug {
				l.Debugf("pull: %q / %q: block %d hash mismatch\n  have: %x\n  want: %x", p.repoCfg.ID, f.Name, i, hb[i].Hash, f.Blocks[i].Hash)
			}
			return
		}
	}

	t := time.Unix(f.Modified, 0)
	err = os.Chtimes(of.temp, t, t)
	if err != nil {
		l.Infof("chtimes: error: %q / %q: %v", p.repoCfg.ID, f.Name, err)
	}
	if !p.repoCfg.IgnorePerms && protocol.HasPermissionBits(f.Flags) {
		err = os.Chmod(of.temp, os.FileMode(f.Flags&0777))
		if err != nil {
			l.Infof("chmod: error: %q / %q: %v", p.repoCfg.ID, f.Name, err)
		}
	}

	osutil.ShowFile(of.temp)

	if p.versioner != nil {
		err := p.versioner.Archive(of.filepath)
		if err != nil {
			if debug {
				l.Debugf("pull: error: %q / %q: %v", p.repoCfg.ID, f.Name, err)
			}
			return
		}
	}

	if debug {
		l.Debugf("pull: rename %q / %q: %q", p.repoCfg.ID, f.Name, of.filepath)
	}
	if err := osutil.Rename(of.temp, of.filepath); err == nil {
		p.model.updateLocal(p.repoCfg.ID, f)
	} else {
		p.errors++
		l.Infof("rename: error: %q / %q: %v", p.repoCfg.ID, f.Name, err)
	}
}

func invalidateRepo(cfg *config.Configuration, repoID string, err error) {
	for i := range cfg.Repositories {
		repo := &cfg.Repositories[i]
		if repo.ID == repoID {
			repo.Invalid = err.Error()
			return
		}
	}
}
