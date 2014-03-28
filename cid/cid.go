// Package cid provides a manager for mappings between node ID:s and connection ID:s.
package cid

import "sync"

type Map struct {
	sync.Mutex
	toCid  map[string]uint
	toName []string
}

var (
	LocalName      = "<local>"
	LocalID   uint = 0
)

func NewMap() *Map {
	return &Map{
		toCid:  map[string]uint{"<local>": 0},
		toName: []string{"<local>"},
	}
}

func (m *Map) Get(name string) uint {
	m.Lock()
	defer m.Unlock()

	cid, ok := m.toCid[name]
	if ok {
		return cid
	}

	// Find a free slot to get a new ID
	for i, n := range m.toName {
		if n == "" {
			m.toName[i] = name
			m.toCid[name] = uint(i)
			return uint(i)
		}
	}

	// Add it to the end since we didn't find a free slot
	m.toName = append(m.toName, name)
	cid = uint(len(m.toName) - 1)
	m.toCid[name] = cid
	return cid
}

func (m *Map) Name(cid uint) string {
	m.Lock()
	defer m.Unlock()

	return m.toName[cid]
}

func (m *Map) Names() []string {
	m.Lock()

	var names []string
	for _, name := range m.toName {
		if name != "" {
			names = append(names, name)
		}
	}

	m.Unlock()
	return names
}

func (m *Map) Clear(name string) {
	m.Lock()
	cid, ok := m.toCid[name]
	if ok {
		m.toName[cid] = ""
		delete(m.toCid, name)
	}
	m.Unlock()
}
