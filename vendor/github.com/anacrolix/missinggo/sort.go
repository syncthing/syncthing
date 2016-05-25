package missinggo

import (
	"reflect"
	"sort"
)

type sorter struct {
	sl   reflect.Value
	less reflect.Value
}

func (s *sorter) Len() int {
	return s.sl.Len()
}

func (s *sorter) Less(i, j int) bool {
	return s.less.Call([]reflect.Value{
		s.sl.Index(i),
		s.sl.Index(j),
	})[0].Bool()
}

func (s *sorter) Swap(i, j int) {
	t := reflect.New(s.sl.Type().Elem()).Elem()
	t.Set(s.sl.Index(i))
	s.sl.Index(i).Set(s.sl.Index(j))
	s.sl.Index(j).Set(t)
}

func Sort(sl interface{}, less interface{}) interface{} {
	sorter := sorter{
		sl:   reflect.ValueOf(sl),
		less: reflect.ValueOf(less),
	}
	sort.Sort(&sorter)
	return sorter.sl.Interface()
}
