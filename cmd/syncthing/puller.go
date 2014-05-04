package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/calmh/syncthing/buffers"
	"github.com/calmh/syncthing/cid"
	"github.com/calmh/syncthing/protocol"
	"github.com/calmh/syncthing/scanner"
)

type requestResult struct {
	node     string
	file     scanner.File
	filepath string // full filepath name
	offset   int64
	data     []byte
	err      error
}

type openFile struct {
	filepath     string // full filepath name
	temp         string // temporary filename
	availability uint64 // availability bitset
	file         *os.File
	err          error // error when opening or writing to file, all following operations are cancelled
	outstanding  int   // number of requests we still have outstanding
	done         bool  // we have sent all requests for this file
}

type activityMap map[string]int

func (m activityMap) leastBusyNode(availability uint64, cm *cid.Map) string {
	var low int = 2<<30 - 1
	var selected string
	for _, node := range cm.Names() {
		id := cm.Get(node)
		if id == cid.LocalID {
			continue
		}
		usage := m[node]
		if availability&(1<<id) != 0 {
			if usage < low {
				low = usage
				selected = node
			}
		}
	}
	m[selected]++
	return selected
}

func (m activityMap) decrease(node string) {
	m[node]--
}

var errNoNode = errors.New("no available source node")

type puller struct {
	repo              string
	dir               string
	bq                *blockQueue
	model             *Model
	oustandingPerNode activityMap
	openFiles         map[string]openFile
	requestSlots      chan bool
	blocks            chan bqBlock
	requestResults    chan requestResult
}

func newPuller(repo, dir string, model *Model, slots int) *puller {
	p := &puller{
		repo:              repo,
		dir:               dir,
		bq:                newBlockQueue(),
		model:             model,
		oustandingPerNode: make(activityMap),
		openFiles:         make(map[string]openFile),
		requestSlots:      make(chan bool, slots),
		blocks:            make(chan bqBlock),
		requestResults:    make(chan requestResult),
	}

	if slots > 0 {
		// Read/write
		for i := 0; i < slots; i++ {
			p.requestSlots <- true
		}
		if debugPull {
			dlog.Printf("starting puller; repo %q dir %q slots %d", repo, dir, slots)
		}
		go p.run()
	} else {
		// Read only
		if debugPull {
			dlog.Printf("starting puller; repo %q dir %q (read only)", repo, dir)
		}
		go p.runRO()
	}
	return p
}

func (p *puller) run() {
	go func() {
		// fill blocks queue when there are free slots
		for {
			<-p.requestSlots
			b := p.bq.get()
			if debugPull {
				dlog.Printf("filler: queueing %q / %q offset %d copy %d", p.repo, b.file.Name, b.block.Offset, len(b.copy))
			}
			p.blocks <- b
		}
	}()

	walkTicker := time.Tick(time.Duration(cfg.Options.RescanIntervalS) * time.Second)
	timeout := time.Tick(5 * time.Second)
	changed := true

	for {
		// Run the pulling loop as long as there are blocks to fetch
	pull:
		for {
			select {
			case res := <-p.requestResults:
				p.model.setState(p.repo, RepoSyncing)
				changed = true
				p.requestSlots <- true
				p.handleRequestResult(res)

			case b := <-p.blocks:
				p.model.setState(p.repo, RepoSyncing)
				changed = true
				if p.handleBlock(b) {
					// Block was fully handled, free up the slot
					p.requestSlots <- true
				}

			case <-timeout:
				if len(p.openFiles) == 0 && p.bq.empty() {
					// Nothing more to do for the moment
					break pull
				}
				if debugPull {
					dlog.Printf("%q: idle but have %d open files", p.repo, len(p.openFiles))
					i := 5
					for _, f := range p.openFiles {
						dlog.Printf("  %v", f)
						i--
						if i == 0 {
							break
						}
					}
				}
			}
		}

		if changed {
			p.model.setState(p.repo, RepoCleaning)
			p.fixupDirectories()
			changed = false
		}

		p.model.setState(p.repo, RepoIdle)

		// Do a rescan if it's time for it
		select {
		case <-walkTicker:
			if debugPull {
				dlog.Printf("%q: time for rescan", p.repo)
			}
			err := p.model.ScanRepo(p.repo)
			if err != nil {
				invalidateRepo(p.repo, err)
				return
			}

		default:
		}

		// Queue more blocks to fetch, if any
		p.queueNeededBlocks()
	}
}

func (p *puller) runRO() {
	walkTicker := time.Tick(time.Duration(cfg.Options.RescanIntervalS) * time.Second)

	for _ = range walkTicker {
		if debugPull {
			dlog.Printf("%q: time for rescan", p.repo)
		}
		err := p.model.ScanRepo(p.repo)
		if err != nil {
			invalidateRepo(p.repo, err)
			return
		}
	}
}

func (p *puller) fixupDirectories() {
	var deleteDirs []string
	filepath.Walk(p.dir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			return nil
		}

		rn, err := filepath.Rel(p.dir, path)
		if err != nil {
			return nil
		}

		if rn == "." {
			return nil
		}

		cur := p.model.CurrentGlobalFile(p.repo, rn)
		if cur.Name != rn {
			// No matching dir in current list; weird
			return nil
		}

		if cur.Flags&protocol.FlagDeleted != 0 {
			if debugPull {
				dlog.Printf("queue delete dir: %v", cur)
			}

			// We queue the directories to delete since we walk the
			// tree in depth first order and need to remove the
			// directories in the opposite order.

			deleteDirs = append(deleteDirs, path)
			return nil
		}

		if cur.Flags&uint32(os.ModePerm) != uint32(info.Mode()&os.ModePerm) {
			os.Chmod(path, os.FileMode(cur.Flags)&os.ModePerm)
			if debugPull {
				dlog.Printf("restored dir flags: %o -> %v", info.Mode()&os.ModePerm, cur)
			}
		}

		if cur.Modified != info.ModTime().Unix() {
			t := time.Unix(cur.Modified, 0)
			os.Chtimes(path, t, t)
			if debugPull {
				dlog.Printf("restored dir modtime: %d -> %v", info.ModTime().Unix(), cur)
			}
		}

		return nil
	})

	// Delete any queued directories
	for i := len(deleteDirs) - 1; i >= 0; i-- {
		if debugPull {
			dlog.Println("delete dir:", deleteDirs[i])
		}
		err := os.Remove(deleteDirs[i])
		if err != nil {
			warnln(err)
		}
	}
}

func (p *puller) handleRequestResult(res requestResult) {
	p.oustandingPerNode.decrease(res.node)
	f := res.file

	of, ok := p.openFiles[f.Name]
	if !ok || of.err != nil {
		// no entry in openFiles means there was an error and we've cancelled the operation
		return
	}

	_, of.err = of.file.WriteAt(res.data, res.offset)
	buffers.Put(res.data)

	of.outstanding--
	p.openFiles[f.Name] = of

	if debugPull {
		dlog.Printf("pull: wrote %q / %q offset %d outstanding %d done %v", p.repo, f.Name, res.offset, of.outstanding, of.done)
	}

	if of.done && of.outstanding == 0 {
		p.closeFile(f)
	}
}

// handleBlock fulfills the block request by copying, ignoring or fetching
// from the network. Returns true if the block was fully handled
// synchronously, i.e. if the slot can be reused.
func (p *puller) handleBlock(b bqBlock) bool {
	f := b.file

	// For directories, simply making sure they exist is enough
	if f.Flags&protocol.FlagDirectory != 0 {
		path := filepath.Join(p.dir, f.Name)
		_, err := os.Stat(path)
		if err != nil && os.IsNotExist(err) {
			os.MkdirAll(path, 0777)
		}
		p.model.updateLocal(p.repo, f)
		return true
	}

	of, ok := p.openFiles[f.Name]
	of.done = b.last

	if !ok {
		if debugPull {
			dlog.Printf("pull: %q: opening file %q", p.repo, f.Name)
		}

		of.availability = uint64(p.model.repoFiles[p.repo].Availability(f.Name))
		of.filepath = filepath.Join(p.dir, f.Name)
		of.temp = filepath.Join(p.dir, defTempNamer.TempName(f.Name))

		dirName := filepath.Dir(of.filepath)
		_, err := os.Stat(dirName)
		if err != nil {
			err = os.MkdirAll(dirName, 0777)
		}
		if err != nil {
			dlog.Printf("pull: error: %q / %q: %v", p.repo, f.Name, err)
		}

		of.file, of.err = os.Create(of.temp)
		if of.err != nil {
			if debugPull {
				dlog.Printf("pull: error: %q / %q: %v", p.repo, f.Name, of.err)
			}
			if !b.last {
				p.openFiles[f.Name] = of
			}
			return true
		}
		defTempNamer.Hide(of.temp)
	}

	if of.err != nil {
		// We have already failed this file.
		if debugPull {
			dlog.Printf("pull: error: %q / %q has already failed: %v", p.repo, f.Name, of.err)
		}
		if b.last {
			dlog.Printf("pull: removing failed file %q / %q", p.repo, f.Name)
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

	if debugPull {
		dlog.Printf("pull: copying %d blocks for %q / %q", len(b.copy), p.repo, f.Name)
	}

	var exfd *os.File
	exfd, of.err = os.Open(of.filepath)
	if of.err != nil {
		if debugPull {
			dlog.Printf("pull: error: %q / %q: %v", p.repo, f.Name, of.err)
		}
		of.file.Close()
		of.file = nil

		p.openFiles[f.Name] = of
		return
	}
	defer exfd.Close()

	for _, b := range b.copy {
		bs := buffers.Get(int(b.Size))
		_, of.err = exfd.ReadAt(bs, b.Offset)
		if of.err == nil {
			_, of.err = of.file.WriteAt(bs, b.Offset)
		}
		buffers.Put(bs)
		if of.err != nil {
			if debugPull {
				dlog.Printf("pull: error: %q / %q: %v", p.repo, f.Name, of.err)
			}
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

	node := p.oustandingPerNode.leastBusyNode(of.availability, p.model.cm)
	if len(node) == 0 {
		of.err = errNoNode
		if of.file != nil {
			of.file.Close()
			of.file = nil
			os.Remove(of.temp)
		}
		if b.last {
			delete(p.openFiles, f.Name)
		} else {
			p.openFiles[f.Name] = of
		}
		return true
	}

	of.outstanding++
	p.openFiles[f.Name] = of

	go func(node string, b bqBlock) {
		if debugPull {
			dlog.Printf("pull: requesting %q / %q offset %d size %d from %q outstanding %d", p.repo, f.Name, b.block.Offset, b.block.Size, node, of.outstanding)
		}

		bs, err := p.model.requestGlobal(node, p.repo, f.Name, b.block.Offset, int(b.block.Size), nil)
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

	if f.Flags&protocol.FlagDeleted != 0 {
		if debugPull {
			dlog.Printf("pull: delete %q", f.Name)
		}
		os.Remove(of.temp)
		os.Remove(of.filepath)
	} else {
		if debugPull {
			dlog.Printf("pull: no blocks to fetch and nothing to copy for %q / %q", p.repo, f.Name)
		}
		t := time.Unix(f.Modified, 0)
		os.Chtimes(of.temp, t, t)
		os.Chmod(of.temp, os.FileMode(f.Flags&0777))
		defTempNamer.Show(of.temp)
		Rename(of.temp, of.filepath)
	}
	delete(p.openFiles, f.Name)
	p.model.updateLocal(p.repo, f)
}

func (p *puller) queueNeededBlocks() {
	queued := 0
	for _, f := range p.model.NeedFilesRepo(p.repo) {
		lf := p.model.CurrentRepoFile(p.repo, f.Name)
		have, need := scanner.BlockDiff(lf.Blocks, f.Blocks)
		if debugNeed {
			dlog.Printf("need:\n  local: %v\n  global: %v\n  haveBlocks: %v\n  needBlocks: %v", lf, f, have, need)
		}
		queued++
		p.bq.put(bqAdd{
			file: f,
			have: have,
			need: need,
		})
	}
	if debugPull && queued > 0 {
		dlog.Printf("%q: queued %d blocks", p.repo, queued)
	}
}

func (p *puller) closeFile(f scanner.File) {
	if debugPull {
		dlog.Printf("pull: closing %q / %q", p.repo, f.Name)
	}

	of := p.openFiles[f.Name]
	of.file.Close()
	defer os.Remove(of.temp)

	delete(p.openFiles, f.Name)

	fd, err := os.Open(of.temp)
	if err != nil {
		if debugPull {
			dlog.Printf("pull: error: %q / %q: %v", p.repo, f.Name, err)
		}
		return
	}
	hb, _ := scanner.Blocks(fd, BlockSize)
	fd.Close()

	if l0, l1 := len(hb), len(f.Blocks); l0 != l1 {
		if debugPull {
			dlog.Printf("pull: %q / %q: nblocks %d != %d", p.repo, f.Name, l0, l1)
		}
		return
	}

	for i := range hb {
		if bytes.Compare(hb[i].Hash, f.Blocks[i].Hash) != 0 {
			dlog.Printf("pull: %q / %q: block %d hash mismatch", p.repo, f.Name, i)
			return
		}
	}

	t := time.Unix(f.Modified, 0)
	os.Chtimes(of.temp, t, t)
	os.Chmod(of.temp, os.FileMode(f.Flags&0777))
	defTempNamer.Show(of.temp)
	if debugPull {
		dlog.Printf("pull: rename %q / %q: %q", p.repo, f.Name, of.filepath)
	}
	if err := Rename(of.temp, of.filepath); err == nil {
		p.model.updateLocal(p.repo, f)
	} else {
		dlog.Printf("pull: error: %q / %q: %v", p.repo, f.Name, err)
	}
}
