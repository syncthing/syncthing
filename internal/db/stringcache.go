package db

type stringCache struct {
	strings map[string]int32
	indexes []string
}

func newStringCache() *stringCache {
	return &stringCache{
		strings: make(map[string]int32),
	}
}

func (s *stringCache) Index(val string) int32 {
	idx, ok := s.strings[val]
	if !ok {
		idx = int32(len(s.indexes))
		s.indexes = append(s.indexes, val)
		s.strings[val] = idx
	}
	return idx
}

func (s *stringCache) Lookup(key int32) string {
	return s.indexes[key]
}
