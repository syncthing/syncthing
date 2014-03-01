package cid

type Map struct {
	toCid  map[string]int
	toName []string
}

func NewMap() *Map {
	return &Map{
		toCid: make(map[string]int),
	}
}

func (m *Map) Get(name string) int {
	cid, ok := m.toCid[name]
	if ok {
		return cid
	}

	// Find a free slot to get a new ID
	for i, n := range m.toName {
		if n == "" {
			m.toName[i] = name
			m.toCid[name] = i
			return i
		}
	}

	// Add it to the end since we didn't find a free slot
	m.toName = append(m.toName, name)
	cid = len(m.toName) - 1
	m.toCid[name] = cid
	return cid
}

func (m *Map) Clear(name string) {
	cid, ok := m.toCid[name]
	if ok {
		m.toName[cid] = ""
		delete(m.toCid, name)
	}
}
