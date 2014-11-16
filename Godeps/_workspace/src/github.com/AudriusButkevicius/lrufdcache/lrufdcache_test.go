package lrufdcache

import (
	"io/ioutil"
	"os"
	"sync"
	"time"

	"testing"
)

func TestNoopReadFailsOnClosed(t *testing.T) {
	fd, err := ioutil.TempFile("", "fdcache")
	if err != nil {
		t.Fatal(err)
		return
	}
	fd.WriteString("test")
	fd.Close()
	buf := make([]byte, 4)
	defer os.Remove(fd.Name())

	_, err = fd.ReadAt(buf, 0)
	if err == nil {
		t.Fatal("Expected error")
	}
}

func TestSingleFileEviction(t *testing.T) {
	c := NewCache(1)

	wg := sync.WaitGroup{}

	fd, err := ioutil.TempFile("", "fdcache")
	if err != nil {
		t.Fatal(err)
		return
	}
	fd.WriteString("test")
	fd.Close()
	buf := make([]byte, 4)
	defer os.Remove(fd.Name())

	for k := 0; k < 100; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			cfd, err := c.Open(fd.Name())
			if err != nil {
				t.Fatal(err)
				return
			}
			defer cfd.Close()

			_, err = cfd.ReadAt(buf, 0)
			if err != nil {
				t.Fatal(err)
			}
		}()
	}

	wg.Wait()
}

func TestMultifileEviction(t *testing.T) {
	c := NewCache(1)

	wg := sync.WaitGroup{}

	for k := 0; k < 100; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			fd, err := ioutil.TempFile("", "fdcache")
			if err != nil {
				t.Fatal(err)
				return
			}
			fd.WriteString("test")
			fd.Close()
			buf := make([]byte, 4)
			defer os.Remove(fd.Name())

			cfd, err := c.Open(fd.Name())
			if err != nil {
				t.Fatal(err)
				return
			}
			defer cfd.Close()

			_, err = cfd.ReadAt(buf, 0)
			if err != nil {
				t.Fatal(err)
			}
		}()
	}

	wg.Wait()
}

func TestMixedEviction(t *testing.T) {
	c := NewCache(1)

	wg := sync.WaitGroup{}
	wg2 := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			fd, err := ioutil.TempFile("", "fdcache")
			if err != nil {
				t.Fatal(err)
				return
			}
			fd.WriteString("test")
			fd.Close()
			buf := make([]byte, 4)

			for k := 0; k < 100; k++ {
				wg.Add(1)
				go func() {
					defer wg.Done()

					cfd, err := c.Open(fd.Name())
					if err != nil {
						t.Fatal(err)
						return
					}
					defer cfd.Close()

					_, err = cfd.ReadAt(buf, 0)
					if err != nil {
						t.Fatal(err)
					}
				}()
			}
		}()
	}

	wg2.Wait()
	wg.Wait()
}

func TestLimit(t *testing.T) {
	testcase := 50
	fd, err := ioutil.TempFile("", "fdcache")
	if err != nil {
		t.Fatal(err)
		return
	}
	fd.Close()
	defer os.Remove(fd.Name())

	c := NewCache(testcase)
	fds := make([]*CachedFile, testcase*2)
	for i := 0; i < testcase*2; i++ {
		fd, err := ioutil.TempFile("", "fdcache")
		if err != nil {
			t.Fatal(err)
			return
		}
		fd.WriteString("test")
		fd.Close()
		defer os.Remove(fd.Name())

		nfd, err := c.Open(fd.Name())
		if err != nil {
			t.Fatal(err)
			return
		}
		fds = append(fds, nfd)
		nfd.Close()
	}

	// Allow closes to happen
	time.Sleep(time.Millisecond * 100)

	buf := make([]byte, 4)
	ok := 0
	for _, fd := range fds {
		if fd == nil {
			continue
		}
		_, err := fd.ReadAt(buf, 0)
		if err == nil {
			ok++
		}
	}

	if ok > testcase {
		t.Fatal("More than", testcase, "fds open")
	}
}
