// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestRealCase(t *testing.T) {
	// Verify realCase lookups on various underlying filesystems.

	t.Run("fake-sensitive", func(t *testing.T) {
		testRealCase(t, newFakeFilesystem(t.Name()))
	})
	t.Run("fake-insensitive", func(t *testing.T) {
		testRealCase(t, newFakeFilesystem(t.Name()+"?insens=true"))
	})
	t.Run("actual", func(t *testing.T) {
		fsys, _ := setup(t)
		testRealCase(t, fsys)
	})
}

func newCaseFilesystem(fsys Filesystem) *caseFilesystem {
	return globalCaseFilesystemRegistry.get(fsys).(*caseFilesystem)
}

func testRealCase(t *testing.T, fsys Filesystem) {
	testFs := newCaseFilesystem(fsys)
	comps := []string{"Foo", "bar", "BAZ", "bAs"}
	path := filepath.Join(comps...)
	testFs.MkdirAll(filepath.Join(comps[:len(comps)-1]...), 0777)
	fd, err := testFs.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	for i, tc := range []struct {
		in  string
		len int
	}{
		{path, 4},
		{strings.ToLower(path), 4},
		{strings.ToUpper(path), 4},
		{"foo", 1},
		{"FOO", 1},
		{"foO", 1},
		{filepath.Join("Foo", "bar"), 2},
		{filepath.Join("Foo", "bAr"), 2},
		{filepath.Join("FoO", "bar"), 2},
		{filepath.Join("foo", "bar", "BAZ"), 3},
		{filepath.Join("Foo", "bar", "bAz"), 3},
		{filepath.Join("foo", "bar", "BAZ"), 3}, // Repeat on purpose
	} {
		out, err := testFs.realCase(tc.in)
		if err != nil {
			t.Error(err)
		} else if exp := filepath.Join(comps[:tc.len]...); out != exp {
			t.Errorf("tc %v: Expected %v, got %v", i, exp, out)
		}
	}
}

func TestRealCaseSensitive(t *testing.T) {
	// Verify that realCase returns the best on-disk case for case sensitive
	// systems. Test is skipped if the underlying fs is insensitive.

	t.Run("fake-sensitive", func(t *testing.T) {
		testRealCaseSensitive(t, newFakeFilesystem(t.Name()))
	})
	t.Run("actual", func(t *testing.T) {
		fsys, _ := setup(t)
		testRealCaseSensitive(t, fsys)
	})
}

func testRealCaseSensitive(t *testing.T, fsys Filesystem) {
	testFs := newCaseFilesystem(fsys)

	names := make([]string, 2)
	names[0] = "foo"
	names[1] = strings.ToUpper(names[0])
	for _, n := range names {
		if err := testFs.MkdirAll(n, 0777); err != nil {
			if IsErrCaseConflict(err) {
				t.Skip("Filesystem is case-insensitive")
			}
			t.Fatal(err)
		}
	}

	for _, n := range names {
		if rn, err := testFs.realCase(n); err != nil {
			t.Error(err)
		} else if rn != n {
			t.Errorf("Got %v, expected %v", rn, n)
		}
	}
}

func TestCaseFSStat(t *testing.T) {
	// Verify that a Stat() lookup behaves in a case sensitive manner
	// regardless of the underlying fs.

	t.Run("fake-sensitive", func(t *testing.T) {
		testCaseFSStat(t, newFakeFilesystem(t.Name()))
	})
	t.Run("fake-insensitive", func(t *testing.T) {
		testCaseFSStat(t, newFakeFilesystem(t.Name()+"?insens=true"))
	})
	t.Run("actual", func(t *testing.T) {
		fsys, _ := setup(t)
		testCaseFSStat(t, fsys)
	})
}

func testCaseFSStat(t *testing.T, fsys Filesystem) {
	fd, err := fsys.Create("foo")
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	// Check if the underlying fs is sensitive or not
	sensitive := true
	if _, err = fsys.Stat("FOO"); err == nil {
		sensitive = false
	}

	testFs := newCaseFilesystem(fsys)
	_, err = testFs.Stat("FOO")
	if sensitive {
		if IsNotExist(err) {
			t.Log("pass: case sensitive underlying fs")
		} else {
			t.Error("expected NotExist, not", err, "for sensitive fs")
		}
	} else if IsErrCaseConflict(err) {
		t.Log("pass: case insensitive underlying fs")
	} else {
		t.Error("expected ErrCaseConflict, not", err, "for insensitive fs")
	}
}

func BenchmarkWalkCaseFakeFS100k(b *testing.B) {
	const entries = 100_000
	fsys, paths, err := fakefsForBenchmark(entries, 0)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("rawfs", func(b *testing.B) {
		var fakefs *fakeFS
		if ffs, ok := unwrapFilesystem(fsys, filesystemWrapperTypeNone); ok {
			fakefs = ffs.(*fakeFS)
		}
		fakefs.resetCounters()
		benchmarkWalkFakeFS(b, fsys, paths, 0, "")
		fakefs.reportMetricsPerOp(b)
		fakefs.reportMetricsPer(b, entries, "entry")
		b.ReportAllocs()
	})
	b.Run("casefs", func(b *testing.B) {
		// Construct the casefs manually or it will get cached and the benchmark is invalid.
		casefs := &caseFilesystem{
			Filesystem: fsys,
			realCaser:  newDefaultRealCaser(fsys),
		}
		var fakefs *fakeFS
		if ffs, ok := unwrapFilesystem(fsys, filesystemWrapperTypeNone); ok {
			fakefs = ffs.(*fakeFS)
		}
		fakefs.resetCounters()
		benchmarkWalkFakeFS(b, casefs, paths, 0, "")
		fakefs.reportMetricsPerOp(b)
		fakefs.reportMetricsPer(b, entries, "entry")
		b.ReportAllocs()
	})
	var otherOpPath string
	sep := string(PathSeparator)
	longest := 0
	for _, p := range paths {
		if length := len(strings.Split(p, sep)); length > longest {
			otherOpPath = p
			longest = length
		}
	}
	otherOpEvery := 1000
	b.Run(fmt.Sprintf("casefs-otherOpEvery%v", otherOpEvery), func(b *testing.B) {
		// Construct the casefs manually or it will get cached and the benchmark is invalid.
		casefs := &caseFilesystem{
			Filesystem: fsys,
			realCaser:  newDefaultRealCaser(fsys),
		}
		var fakefs *fakeFS
		if ffs, ok := unwrapFilesystem(fsys, filesystemWrapperTypeNone); ok {
			fakefs = ffs.(*fakeFS)
		}
		fakefs.resetCounters()
		benchmarkWalkFakeFS(b, casefs, paths, otherOpEvery, otherOpPath)
		fakefs.reportMetricsPerOp(b)
		fakefs.reportMetricsPer(b, entries, "entry")
		b.ReportAllocs()
	})
}

func benchmarkWalkFakeFS(b *testing.B, fsys Filesystem, paths []string, otherOpEvery int, otherOpPath string) {
	// Simulate a scanner pass over the filesystem. First walk it to
	// discover all names, then stat each name individually to check if it's
	// been deleted or not (pretending that they all existed in the
	// database).

	var ms0 runtime.MemStats
	runtime.ReadMemStats(&ms0)
	t0 := time.Now()

	for i := 0; i < b.N; i++ {
		if err := doubleWalkFSWithOtherOps(fsys, paths, otherOpEvery, otherOpPath); err != nil {
			b.Fatal(err)
		}
	}

	t1 := time.Now()
	var ms1 runtime.MemStats
	runtime.ReadMemStats(&ms1)

	// We add metrics per path entry
	b.ReportMetric(float64(t1.Sub(t0))/float64(b.N)/float64(len(paths)), "ns/entry")
	b.ReportMetric(float64(ms1.Mallocs-ms0.Mallocs)/float64(b.N)/float64(len(paths)), "allocs/entry")
	b.ReportMetric(float64(ms1.TotalAlloc-ms0.TotalAlloc)/float64(b.N)/float64(len(paths)), "B/entry")
}

func TestStressCaseFS(t *testing.T) {
	// Exercise a bunch of parallel operations for stressing out race
	// conditions in the realnamer cache etc.

	const limit = 10 * time.Second
	if testing.Short() {
		t.Skip("long test")
	}

	fsys, paths, err := fakefsForBenchmark(10_000, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < runtime.NumCPU()/2+1; i++ {
		t.Run(fmt.Sprintf("walker-%d", i), func(t *testing.T) {
			// Walk the filesystem and stat everything
			t.Parallel()
			t0 := time.Now()
			for time.Since(t0) < limit {
				if err := doubleWalkFS(fsys, paths); err != nil {
					t.Fatal(err)
				}
			}
		})
		t.Run(fmt.Sprintf("toucher-%d", i), func(t *testing.T) {
			// Touch all the things
			t.Parallel()
			t0 := time.Now()
			for time.Since(t0) < limit {
				for _, p := range paths {
					now := time.Now()
					if err := fsys.Chtimes(p, now, now); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func doubleWalkFS(fsys Filesystem, paths []string) error {
	return doubleWalkFSWithOtherOps(fsys, paths, 0, "")
}

func doubleWalkFSWithOtherOps(fsys Filesystem, paths []string, otherOpEvery int, otherOpPath string) error {
	i := 0
	if err := fsys.Walk("/", func(path string, info FileInfo, err error) error {
		i++
		if otherOpEvery != 0 && i%otherOpEvery == 0 {
			// l.Infoln("AAA", otherOpPath)
			if _, err := fsys.Lstat(otherOpPath); err != nil {
				return err
			}
		}
		// l.Infoln("CCC", path)
		return err
	}); err != nil {
		return err
	}

	for _, p := range paths {
		for p != "." {
			i++
			if otherOpEvery != 0 && i%otherOpEvery == 0 {
				if _, err := fsys.Lstat(otherOpPath); err != nil {
					// l.Infoln("AAA", otherOpPath)
					return err
				}
			}
			// l.Infoln("CCC", p)
			if _, err := fsys.Lstat(p); err != nil {
				return err
			}
			p = filepath.Dir(p)
		}
	}
	return nil
}

func fakefsForBenchmark(nfiles int, latency time.Duration) (Filesystem, []string, error) {
	fsys := NewFilesystem(FilesystemTypeFake, fmt.Sprintf("fakefsForBenchmark?files=%d&insens=true&latency=%s", nfiles, latency), testOpts...)

	var paths []string
	if err := fsys.Walk("/", func(path string, info FileInfo, err error) error {
		paths = append(paths, path)
		return err
	}); err != nil {
		return nil, nil, err
	}
	if len(paths) < nfiles {
		return nil, nil, errors.New("didn't find enough stuff")
	}

	sort.Strings(paths)

	return fsys, paths, nil
}
