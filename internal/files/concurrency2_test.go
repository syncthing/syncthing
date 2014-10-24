// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package files_test

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/files"
	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestLongConcurrent(t *testing.T) {
	if testing.Short() || runtime.GOMAXPROCS(-1) < 4 {
		return
	}

	os.RemoveAll("/tmp/test.db")
	db, err := leveldb.OpenFile("/tmp/test.db", &opt.Options{CachedOpenFiles: 100})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})

	log.Println("preparing")
	var wg sync.WaitGroup
	for i := 0; i < runtime.GOMAXPROCS(-1); i++ {
		i := i
		rem0, rem1 := generateFiles()
		wg.Add(1)
		go func() {
			defer wg.Done()
			longConcurrentTest(db, fmt.Sprintf("folder%d", i), rem0, rem1, start)
		}()
	}

	log.Println("starting")
	close(start)
	wg.Wait()
}

func generateFiles() ([]protocol.FileInfo, []protocol.FileInfo) {
	var rem0, rem1 fileList

	for i := 0; i < 10000; i++ {
		n := rand.Int()
		rem0 = append(rem0, protocol.FileInfo{
			Name:    fmt.Sprintf("path/path/path/path/path/path/path%d/path%d/path%d/file%d", n, n, n, n),
			Version: uint64(rand.Int63()),
			Blocks:  genBlocks(rand.Intn(25)),
			Flags:   uint32(rand.Int31()),
		})
	}

	for i := 0; i < 10000; i++ {
		if i%2 == 0 {
			// Same file as rem0, randomly newer or older
			f := rem0[i]
			f.Version = uint64(rand.Int63())
			rem1 = append(rem1, f)
		} else {
			// Different file
			n := rand.Int()
			f := protocol.FileInfo{
				Name:    fmt.Sprintf("path/path/path/path/path/path/path%d/path%d/path%d/file%d", n, n, n, n),
				Version: uint64(rand.Int63()),
				Blocks:  genBlocks(rand.Intn(25)),
				Flags:   uint32(rand.Int31()),
			}
			rem1 = append(rem1, f)
		}
	}

	return rem0, rem1
}

func longConcurrentTest(db *leveldb.DB, folder string, rem0, rem1 []protocol.FileInfo, start chan struct{}) {
	s := files.NewSet(folder, db)

	<-start

	t0 := time.Now()
	cont := func() bool {
		return time.Since(t0) < 60*time.Second
	}

	log.Println(folder, "start")

	var wg sync.WaitGroup

	// Fast updater

	wg.Add(1)
	go func() {
		defer wg.Done()
		for cont() {
			log.Println(folder, "u0")
			for i := 0; i < 10000; i += 250 {
				s.Update(remoteDevice0, rem0[i:i+250])
			}
			time.Sleep(25 * time.Millisecond)
			s.Replace(remoteDevice0, nil)
			time.Sleep(25 * time.Millisecond)
		}
	}()

	// Fast updater

	wg.Add(1)
	go func() {
		defer wg.Done()
		for cont() {
			log.Println(folder, "u1")
			for i := 0; i < 10000; i += 250 {
				s.Update(remoteDevice1, rem1[i:i+250])
			}
			time.Sleep(25 * time.Millisecond)
			s.Replace(remoteDevice1, nil)
			time.Sleep(25 * time.Millisecond)
		}
	}()

	// Fast need list

	wg.Add(1)
	go func() {
		defer wg.Done()
		for cont() {
			needList(s, protocol.LocalDeviceID)
			time.Sleep(25 * time.Millisecond)
		}
	}()

	// Fast global list

	wg.Add(1)
	go func() {
		defer wg.Done()
		for cont() {
			globalList(s)
			time.Sleep(25 * time.Millisecond)
		}
	}()

	// Long running need lists

	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(25 * time.Millisecond)
			wg.Add(1)
			go func() {
				defer wg.Done()
				for cont() {
					s.WithNeed(protocol.LocalDeviceID, func(intf protocol.FileIntf) bool {
						time.Sleep(50 * time.Millisecond)
						return cont()
					})
				}
			}()
		}
	}()

	// Long running global lists

	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(25 * time.Millisecond)
			wg.Add(1)
			go func() {
				defer wg.Done()
				for cont() {
					s.WithGlobal(func(intf protocol.FileIntf) bool {
						time.Sleep(50 * time.Millisecond)
						return cont()
					})
				}
			}()
		}
	}()

	wg.Wait()

	log.Println(folder, "done")
}
