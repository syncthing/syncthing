package fileset

import (
	"fmt"
	"reflect"
	"testing"
)

func TestGlobalSet(t *testing.T) {
	m := NewSet()

	local := []File{
		File{Key{"a", 1000}, 0, 0, nil},
		File{Key{"b", 1000}, 0, 0, nil},
		File{Key{"c", 1000}, 0, 0, nil},
		File{Key{"d", 1000}, 0, 0, nil},
	}

	remote := []File{
		File{Key{"a", 1000}, 0, 0, nil},
		File{Key{"b", 1001}, 0, 0, nil},
		File{Key{"c", 1002}, 0, 0, nil},
		File{Key{"e", 1000}, 0, 0, nil},
	}

	expectedGlobal := map[string]Key{
		"a": local[0].Key,
		"b": remote[1].Key,
		"c": remote[2].Key,
		"d": local[3].Key,
		"e": remote[3].Key,
	}

	m.SetLocal(local)
	m.SetRemote(1, remote)

	if !reflect.DeepEqual(m.globalKey, expectedGlobal) {
		t.Errorf("Global incorrect;\n%v !=\n%v", m.globalKey, expectedGlobal)
	}

	if lb := len(m.files); lb != 7 {
		t.Errorf("Num files incorrect %d != 7\n%v", lb, m.files)
	}
}

func BenchmarkSetLocal10k(b *testing.B) {
	m := NewSet()

	var local []File
	for i := 0; i < 10000; i++ {
		local = append(local, File{Key{fmt.Sprintf("file%d"), 1000}, 0, 0, nil})
	}

	var remote []File
	for i := 0; i < 10000; i++ {
		remote = append(remote, File{Key{fmt.Sprintf("file%d"), 1000}, 0, 0, nil})
	}

	m.SetRemote(1, remote)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.SetLocal(local)
	}
}

func BenchmarkSetLocal10(b *testing.B) {
	m := NewSet()

	var local []File
	for i := 0; i < 10; i++ {
		local = append(local, File{Key{fmt.Sprintf("file%d"), 1000}, 0, 0, nil})
	}

	var remote []File
	for i := 0; i < 10000; i++ {
		remote = append(remote, File{Key{fmt.Sprintf("file%d"), 1000}, 0, 0, nil})
	}

	m.SetRemote(1, remote)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.SetLocal(local)
	}
}

func BenchmarkAddLocal10k(b *testing.B) {
	m := NewSet()

	var local []File
	for i := 0; i < 10000; i++ {
		local = append(local, File{Key{fmt.Sprintf("file%d"), 1000}, 0, 0, nil})
	}

	var remote []File
	for i := 0; i < 10000; i++ {
		remote = append(remote, File{Key{fmt.Sprintf("file%d"), 1000}, 0, 0, nil})
	}

	m.SetRemote(1, remote)
	m.SetLocal(local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		for j := range local {
			local[j].Key.Version++
		}
		b.StartTimer()
		m.AddLocal(local)
	}
}

func BenchmarkAddLocal10(b *testing.B) {
	m := NewSet()

	var local []File
	for i := 0; i < 10; i++ {
		local = append(local, File{Key{fmt.Sprintf("file%d"), 1000}, 0, 0, nil})
	}

	var remote []File
	for i := 0; i < 10000; i++ {
		remote = append(remote, File{Key{fmt.Sprintf("file%d"), 1000}, 0, 0, nil})
	}

	m.SetRemote(1, remote)
	m.SetLocal(local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range local {
			local[j].Key.Version++
		}
		m.AddLocal(local)
	}
}

func TestGlobalReset(t *testing.T) {
	m := NewSet()

	local := []File{
		File{Key{"a", 1000}, 0, 0, nil},
		File{Key{"b", 1000}, 0, 0, nil},
		File{Key{"c", 1000}, 0, 0, nil},
		File{Key{"d", 1000}, 0, 0, nil},
	}

	remote := []File{
		File{Key{"a", 1000}, 0, 0, nil},
		File{Key{"b", 1001}, 0, 0, nil},
		File{Key{"c", 1002}, 0, 0, nil},
		File{Key{"e", 1000}, 0, 0, nil},
	}

	expectedGlobalKey := map[string]Key{
		"a": local[0].Key,
		"b": local[1].Key,
		"c": local[2].Key,
		"d": local[3].Key,
	}

	m.SetLocal(local)
	m.SetRemote(1, remote)
	m.SetRemote(1, nil)

	if !reflect.DeepEqual(m.globalKey, expectedGlobalKey) {
		t.Errorf("Global incorrect;\n%v !=\n%v", m.globalKey, expectedGlobalKey)
	}

	if lb := len(m.files); lb != 4 {
		t.Errorf("Num files incorrect %d != 4\n%v", lb, m.files)
	}
}

func TestNeed(t *testing.T) {
	m := NewSet()

	local := []File{
		File{Key{"a", 1000}, 0, 0, nil},
		File{Key{"b", 1000}, 0, 0, nil},
		File{Key{"c", 1000}, 0, 0, nil},
		File{Key{"d", 1000}, 0, 0, nil},
	}

	remote := []File{
		File{Key{"a", 1000}, 0, 0, nil},
		File{Key{"b", 1001}, 0, 0, nil},
		File{Key{"c", 1002}, 0, 0, nil},
		File{Key{"e", 1000}, 0, 0, nil},
	}

	shouldNeed := []File{
		File{Key{"b", 1001}, 0, 0, nil},
		File{Key{"c", 1002}, 0, 0, nil},
		File{Key{"e", 1000}, 0, 0, nil},
	}

	m.SetLocal(local)
	m.SetRemote(1, remote)

	need := m.Need(0)
	if !reflect.DeepEqual(need, shouldNeed) {
		t.Errorf("Need incorrect;\n%v !=\n%v", need, shouldNeed)
	}
}
