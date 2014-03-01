package fileset

import "sync"

type File struct {
	Key      Key
	Modified int64
	Flags    uint32
	Data     interface{}
}

type Key struct {
	Name    string
	Version uint32
}

type fileRecord struct {
	Usage int
	File  File
}

type bitset uint64

func (a Key) newerThan(b Key) bool {
	return a.Version > b.Version
}

type Set struct {
	mutex              sync.RWMutex
	files              map[Key]fileRecord
	remoteKey          [64]map[string]Key
	globalAvailability map[string]bitset
	globalKey          map[string]Key
}

func NewSet() *Set {
	var m = Set{
		files:              make(map[Key]fileRecord),
		globalAvailability: make(map[string]bitset),
		globalKey:          make(map[string]Key),
	}
	return &m
}

func (m *Set) AddLocal(fs []File) {
	m.mutex.Lock()
	m.unlockedAddRemote(0, fs)
	m.mutex.Unlock()
}

func (m *Set) SetLocal(fs []File) {
	m.mutex.Lock()
	m.unlockedSetRemote(0, fs)
	m.mutex.Unlock()
}

func (m *Set) AddRemote(cid uint, fs []File) {
	if cid < 1 || cid > 63 {
		panic("Connection ID must be in the range 1 - 63 inclusive")
	}
	m.mutex.Lock()
	m.unlockedAddRemote(cid, fs)
	m.mutex.Unlock()
}

func (m *Set) SetRemote(cid uint, fs []File) {
	if cid < 1 || cid > 63 {
		panic("Connection ID must be in the range 1 - 63 inclusive")
	}
	m.mutex.Lock()
	m.unlockedSetRemote(cid, fs)
	m.mutex.Unlock()
}

func (m *Set) unlockedAddRemote(cid uint, fs []File) {
	remFiles := m.remoteKey[cid]
	for _, f := range fs {
		n := f.Key.Name

		if ck, ok := remFiles[n]; ok && ck == f.Key {
			// The remote already has exactly this file, skip it
			continue
		}

		remFiles[n] = f.Key

		// Keep the block list or increment the usage
		if br, ok := m.files[f.Key]; !ok {
			m.files[f.Key] = fileRecord{
				Usage: 1,
				File:  f,
			}
		} else {
			br.Usage++
			m.files[f.Key] = br
		}

		// Update global view
		gk, ok := m.globalKey[n]
		switch {
		case ok && f.Key == gk:
			av := m.globalAvailability[n]
			av |= 1 << cid
			m.globalAvailability[n] = av
		case f.Key.newerThan(gk):
			m.globalKey[n] = f.Key
			m.globalAvailability[n] = 1 << cid
		}
	}
}

func (m *Set) unlockedSetRemote(cid uint, fs []File) {
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
	m.remoteKey[cid] = make(map[string]Key)

	// Recalculate global based on all remaining remoteKey
	for n := range m.globalKey {
		var nk Key    // newest key
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
	m.unlockedAddRemote(cid, fs)
}

func (m *Set) Need(cid uint) []File {
	var fs []File
	m.mutex.Lock()

	for name, gk := range m.globalKey {
		if gk.newerThan(m.remoteKey[cid][name]) {
			fs = append(fs, m.files[gk].File)
		}
	}

	m.mutex.Unlock()
	return fs
}
