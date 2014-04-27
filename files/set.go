// Package files provides a set type to track local/remote files with newness checks.
package files

import (
	"sync"

	"github.com/calmh/syncthing/cid"
	"github.com/calmh/syncthing/lamport"
	"github.com/calmh/syncthing/protocol"
	"github.com/calmh/syncthing/scanner"
)

type fileRecord struct {
	File   scanner.File
	Usage  int
	Global bool
}

type bitset uint64

type Set struct {
	sync.Mutex
	files              map[key]fileRecord
	remoteKey          [64]map[string]key
	changes            [64]uint64
	globalAvailability map[string]bitset
	globalKey          map[string]key
}

func NewSet() *Set {
	var m = Set{
		files:              make(map[key]fileRecord),
		globalAvailability: make(map[string]bitset),
		globalKey:          make(map[string]key),
	}
	return &m
}

func (m *Set) Replace(id uint, fs []scanner.File) {
	if debug {
		dlog.Printf("Replace(%d, [%d])", id, len(fs))
	}
	if id > 63 {
		panic("Connection ID must be in the range 0 - 63 inclusive")
	}

	m.Lock()
	if len(fs) == 0 || !m.equals(id, fs) {
		m.changes[id]++
		m.replace(id, fs)
	}
	m.Unlock()
}

func (m *Set) ReplaceWithDelete(id uint, fs []scanner.File) {
	if debug {
		dlog.Printf("ReplaceWithDelete(%d, [%d])", id, len(fs))
	}
	if id > 63 {
		panic("Connection ID must be in the range 0 - 63 inclusive")
	}

	m.Lock()
	if len(fs) == 0 || !m.equals(id, fs) {
		m.changes[id]++

		var nf = make(map[string]key, len(fs))
		for _, f := range fs {
			nf[f.Name] = keyFor(f)
		}

		// For previously existing files not in the list, add them to the list
		// with the relevant delete flags etc set. Previously existing files
		// with the delete bit already set are not modified.

		for _, ck := range m.remoteKey[cid.LocalID] {
			if _, ok := nf[ck.Name]; !ok {
				cf := m.files[ck].File
				if cf.Flags&protocol.FlagDeleted != protocol.FlagDeleted {
					cf.Flags |= protocol.FlagDeleted
					cf.Blocks = nil
					cf.Size = 0
					cf.Version = lamport.Default.Tick(cf.Version)
				}
				fs = append(fs, cf)
				if debug {
					dlog.Println("deleted:", ck.Name)
				}
			}
		}

		m.replace(id, fs)
	}
	m.Unlock()
}

func (m *Set) Update(id uint, fs []scanner.File) {
	if debug {
		dlog.Printf("Update(%d, [%d])", id, len(fs))
	}
	m.Lock()
	m.update(id, fs)
	m.changes[id]++
	m.Unlock()
}

func (m *Set) Need(id uint) []scanner.File {
	if debug {
		dlog.Printf("Need(%d)", id)
	}
	m.Lock()
	var fs = make([]scanner.File, 0, len(m.globalKey)/2) // Just a guess, but avoids too many reallocations
	rkID := m.remoteKey[id]
	for gk, gf := range m.files {
		if !gf.Global {
			continue
		}

		file := gf.File
		switch {
		case file.Flags&protocol.FlagDirectory == 0 && gk.newerThan(rkID[gk.Name]):
			fs = append(fs, file)
		case file.Flags&(protocol.FlagDirectory|protocol.FlagDeleted) == protocol.FlagDirectory && gk.newerThan(rkID[gk.Name]):
			fs = append(fs, file)
		}
	}
	m.Unlock()
	return fs
}

func (m *Set) Have(id uint) []scanner.File {
	if debug {
		dlog.Printf("Have(%d)", id)
	}
	var fs = make([]scanner.File, 0, len(m.remoteKey[id]))
	m.Lock()
	for _, rk := range m.remoteKey[id] {
		fs = append(fs, m.files[rk].File)
	}
	m.Unlock()
	return fs
}

func (m *Set) Global() []scanner.File {
	if debug {
		dlog.Printf("Global()")
	}
	m.Lock()
	var fs = make([]scanner.File, 0, len(m.globalKey))
	for _, file := range m.files {
		if file.Global {
			fs = append(fs, file.File)
		}
	}
	m.Unlock()
	return fs
}

func (m *Set) Get(id uint, file string) scanner.File {
	m.Lock()
	defer m.Unlock()
	if debug {
		dlog.Printf("Get(%d, %q)", id, file)
	}
	return m.files[m.remoteKey[id][file]].File
}

func (m *Set) GetGlobal(file string) scanner.File {
	m.Lock()
	defer m.Unlock()
	if debug {
		dlog.Printf("GetGlobal(%q)", file)
	}
	return m.files[m.globalKey[file]].File
}

func (m *Set) Availability(name string) bitset {
	m.Lock()
	defer m.Unlock()
	av := m.globalAvailability[name]
	if debug {
		dlog.Printf("Availability(%q) = %0x", name, av)
	}
	return av
}

func (m *Set) Changes(id uint) uint64 {
	m.Lock()
	defer m.Unlock()
	if debug {
		dlog.Printf("Changes(%d)", id)
	}
	return m.changes[id]
}

func (m *Set) equals(id uint, fs []scanner.File) bool {
	curWithoutDeleted := make(map[string]key)
	for _, k := range m.remoteKey[id] {
		f := m.files[k].File
		if f.Flags&protocol.FlagDeleted == 0 {
			curWithoutDeleted[f.Name] = k
		}
	}
	if len(curWithoutDeleted) != len(fs) {
		return false
	}
	for _, f := range fs {
		if curWithoutDeleted[f.Name] != keyFor(f) {
			return false
		}
	}
	return true
}

func (m *Set) update(cid uint, fs []scanner.File) {
	remFiles := m.remoteKey[cid]
	for _, f := range fs {
		n := f.Name
		fk := keyFor(f)

		if ck, ok := remFiles[n]; ok && ck == fk {
			// The remote already has exactly this file, skip it
			continue
		}

		remFiles[n] = fk

		// Keep the block list or increment the usage
		if br, ok := m.files[fk]; !ok {
			m.files[fk] = fileRecord{
				Usage: 1,
				File:  f,
			}
		} else {
			br.Usage++
			m.files[fk] = br
		}

		// Update global view
		gk, ok := m.globalKey[n]
		switch {
		case ok && fk == gk:
			av := m.globalAvailability[n]
			av |= 1 << cid
			m.globalAvailability[n] = av
		case fk.newerThan(gk):
			if ok {
				f := m.files[gk]
				f.Global = false
				m.files[gk] = f
			}
			f := m.files[fk]
			f.Global = true
			m.files[fk] = f
			m.globalKey[n] = fk
			m.globalAvailability[n] = 1 << cid
		}
	}
}

func (m *Set) replace(cid uint, fs []scanner.File) {
	// Decrement usage for all files belonging to this remote, and remove
	// those that are no longer needed.
	for _, fk := range m.remoteKey[cid] {
		br, ok := m.files[fk]
		switch {
		case ok && br.Usage == 1:
			delete(m.files, fk)
		case ok && br.Usage > 1:
			br.Usage--
			m.files[fk] = br
		}
	}

	// Clear existing remote remoteKey
	m.remoteKey[cid] = make(map[string]key)

	// Recalculate global based on all remaining remoteKey
	for n := range m.globalKey {
		var nk key    // newest key
		var na bitset // newest availability

		for i, rem := range m.remoteKey {
			if rk, ok := rem[n]; ok {
				switch {
				case rk == nk:
					na |= 1 << uint(i)
				case rk.newerThan(nk):
					nk = rk
					na = 1 << uint(i)
				}
			}
		}

		if na != 0 {
			// Someone had the file
			m.globalKey[n] = nk
			m.globalAvailability[n] = na
		} else {
			// Noone had the file
			delete(m.globalKey, n)
			delete(m.globalAvailability, n)
		}
	}

	// Add new remote remoteKey to the mix
	m.update(cid, fs)
}
