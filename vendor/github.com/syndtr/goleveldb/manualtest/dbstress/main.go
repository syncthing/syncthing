package main

import (
	"crypto/rand"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	mrand "math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/table"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	dbPath                 = path.Join(os.TempDir(), "goleveldb-testdb")
	openFilesCacheCapacity = 500
	keyLen                 = 63
	valueLen               = 256
	numKeys                = arrayInt{100000, 1332, 531, 1234, 9553, 1024, 35743}
	httpProf               = "127.0.0.1:5454"
	transactionProb        = 0.5
	enableBlockCache       = false
	enableCompression      = false
	enableBufferPool       = false

	wg         = new(sync.WaitGroup)
	done, fail uint32

	bpool *util.BufferPool
)

type arrayInt []int

func (a arrayInt) String() string {
	var str string
	for i, n := range a {
		if i > 0 {
			str += ","
		}
		str += strconv.Itoa(n)
	}
	return str
}

func (a *arrayInt) Set(str string) error {
	var na arrayInt
	for _, s := range strings.Split(str, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			n, err := strconv.Atoi(s)
			if err != nil {
				return err
			}
			na = append(na, n)
		}
	}
	*a = na
	return nil
}

func init() {
	flag.StringVar(&dbPath, "db", dbPath, "testdb path")
	flag.IntVar(&openFilesCacheCapacity, "openfilescachecap", openFilesCacheCapacity, "open files cache capacity")
	flag.IntVar(&keyLen, "keylen", keyLen, "key length")
	flag.IntVar(&valueLen, "valuelen", valueLen, "value length")
	flag.Var(&numKeys, "numkeys", "num keys")
	flag.StringVar(&httpProf, "httpprof", httpProf, "http pprof listen addr")
	flag.Float64Var(&transactionProb, "transactionprob", transactionProb, "probablity of writes using transaction")
	flag.BoolVar(&enableBufferPool, "enablebufferpool", enableBufferPool, "enable buffer pool")
	flag.BoolVar(&enableBlockCache, "enableblockcache", enableBlockCache, "enable block cache")
	flag.BoolVar(&enableCompression, "enablecompression", enableCompression, "enable block compression")
}

func randomData(dst []byte, ns, prefix byte, i uint32, dataLen int) []byte {
	if dataLen < (2+4+4)*2+4 {
		panic("dataLen is too small")
	}
	if cap(dst) < dataLen {
		dst = make([]byte, dataLen)
	} else {
		dst = dst[:dataLen]
	}
	half := (dataLen - 4) / 2
	if _, err := rand.Reader.Read(dst[2 : half-8]); err != nil {
		panic(err)
	}
	dst[0] = ns
	dst[1] = prefix
	binary.LittleEndian.PutUint32(dst[half-8:], i)
	binary.LittleEndian.PutUint32(dst[half-8:], i)
	binary.LittleEndian.PutUint32(dst[half-4:], util.NewCRC(dst[:half-4]).Value())
	full := half * 2
	copy(dst[half:full], dst[:half])
	if full < dataLen-4 {
		if _, err := rand.Reader.Read(dst[full : dataLen-4]); err != nil {
			panic(err)
		}
	}
	binary.LittleEndian.PutUint32(dst[dataLen-4:], util.NewCRC(dst[:dataLen-4]).Value())
	return dst
}

func dataSplit(data []byte) (data0, data1 []byte) {
	n := (len(data) - 4) / 2
	return data[:n], data[n : n+n]
}

func dataNS(data []byte) byte {
	return data[0]
}

func dataPrefix(data []byte) byte {
	return data[1]
}

func dataI(data []byte) uint32 {
	return binary.LittleEndian.Uint32(data[(len(data)-4)/2-8:])
}

func dataChecksum(data []byte) (uint32, uint32) {
	checksum0 := binary.LittleEndian.Uint32(data[len(data)-4:])
	checksum1 := util.NewCRC(data[:len(data)-4]).Value()
	return checksum0, checksum1
}

func dataPrefixSlice(ns, prefix byte) *util.Range {
	return util.BytesPrefix([]byte{ns, prefix})
}

func dataNsSlice(ns byte) *util.Range {
	return util.BytesPrefix([]byte{ns})
}

type testingStorage struct {
	storage.Storage
}

func (ts *testingStorage) scanTable(fd storage.FileDesc, checksum bool) (corrupted bool) {
	r, err := ts.Open(fd)
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	size, err := r.Seek(0, os.SEEK_END)
	if err != nil {
		log.Fatal(err)
	}

	o := &opt.Options{
		DisableLargeBatchTransaction: true,
		Strict: opt.NoStrict,
	}
	if checksum {
		o.Strict = opt.StrictBlockChecksum | opt.StrictReader
	}
	tr, err := table.NewReader(r, size, fd, nil, bpool, o)
	if err != nil {
		log.Fatal(err)
	}
	defer tr.Release()

	checkData := func(i int, t string, data []byte) bool {
		if len(data) == 0 {
			panic(fmt.Sprintf("[%v] nil data: i=%d t=%s", fd, i, t))
		}

		checksum0, checksum1 := dataChecksum(data)
		if checksum0 != checksum1 {
			atomic.StoreUint32(&fail, 1)
			atomic.StoreUint32(&done, 1)
			corrupted = true

			data0, data1 := dataSplit(data)
			data0c0, data0c1 := dataChecksum(data0)
			data1c0, data1c1 := dataChecksum(data1)
			log.Printf("FATAL: [%v] Corrupted data i=%d t=%s (%#x != %#x): %x(%v) vs %x(%v)",
				fd, i, t, checksum0, checksum1, data0, data0c0 == data0c1, data1, data1c0 == data1c1)
			return true
		}
		return false
	}

	iter := tr.NewIterator(nil, nil)
	defer iter.Release()
	for i := 0; iter.Next(); i++ {
		ukey, _, kt, kerr := parseIkey(iter.Key())
		if kerr != nil {
			atomic.StoreUint32(&fail, 1)
			atomic.StoreUint32(&done, 1)
			corrupted = true

			log.Printf("FATAL: [%v] Corrupted ikey i=%d: %v", fd, i, kerr)
			return
		}
		if checkData(i, "key", ukey) {
			return
		}
		if kt == ktVal && checkData(i, "value", iter.Value()) {
			return
		}
	}
	if err := iter.Error(); err != nil {
		if errors.IsCorrupted(err) {
			atomic.StoreUint32(&fail, 1)
			atomic.StoreUint32(&done, 1)
			corrupted = true

			log.Printf("FATAL: [%v] Corruption detected: %v", fd, err)
		} else {
			log.Fatal(err)
		}
	}

	return
}

func (ts *testingStorage) Remove(fd storage.FileDesc) error {
	if atomic.LoadUint32(&fail) == 1 {
		return nil
	}

	if fd.Type == storage.TypeTable {
		if ts.scanTable(fd, true) {
			return nil
		}
	}
	return ts.Storage.Remove(fd)
}

type latencyStats struct {
	mark          time.Time
	dur, min, max time.Duration
	num           int
}

func (s *latencyStats) start() {
	s.mark = time.Now()
}

func (s *latencyStats) record(n int) {
	if s.mark.IsZero() {
		panic("not started")
	}
	dur := time.Now().Sub(s.mark)
	dur1 := dur / time.Duration(n)
	if dur1 < s.min || s.min == 0 {
		s.min = dur1
	}
	if dur1 > s.max {
		s.max = dur1
	}
	s.dur += dur
	s.num += n
	s.mark = time.Time{}
}

func (s *latencyStats) ratePerSec() int {
	durSec := s.dur / time.Second
	if durSec > 0 {
		return s.num / int(durSec)
	}
	return s.num
}

func (s *latencyStats) avg() time.Duration {
	if s.num > 0 {
		return s.dur / time.Duration(s.num)
	}
	return 0
}

func (s *latencyStats) add(x *latencyStats) {
	if x.min < s.min || s.min == 0 {
		s.min = x.min
	}
	if x.max > s.max {
		s.max = x.max
	}
	s.dur += x.dur
	s.num += x.num
}

func main() {
	flag.Parse()

	if enableBufferPool {
		bpool = util.NewBufferPool(opt.DefaultBlockSize + 128)
	}

	log.Printf("Test DB stored at %q", dbPath)
	if httpProf != "" {
		log.Printf("HTTP pprof listening at %q", httpProf)
		runtime.SetBlockProfileRate(1)
		go func() {
			if err := http.ListenAndServe(httpProf, nil); err != nil {
				log.Fatalf("HTTPPROF: %v", err)
			}
		}()
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	os.RemoveAll(dbPath)
	stor, err := storage.OpenFile(dbPath, false)
	if err != nil {
		log.Fatal(err)
	}
	tstor := &testingStorage{stor}
	defer tstor.Close()

	fatalf := func(err error, format string, v ...interface{}) {
		atomic.StoreUint32(&fail, 1)
		atomic.StoreUint32(&done, 1)
		log.Printf("FATAL: "+format, v...)
		if err != nil && errors.IsCorrupted(err) {
			cerr := err.(*errors.ErrCorrupted)
			if !cerr.Fd.Nil() && cerr.Fd.Type == storage.TypeTable {
				log.Print("FATAL: corruption detected, scanning...")
				if !tstor.scanTable(storage.FileDesc{Type: storage.TypeTable, Num: cerr.Fd.Num}, false) {
					log.Printf("FATAL: unable to find corrupted key/value pair in table %v", cerr.Fd)
				}
			}
		}
		runtime.Goexit()
	}

	if openFilesCacheCapacity == 0 {
		openFilesCacheCapacity = -1
	}
	o := &opt.Options{
		OpenFilesCacheCapacity: openFilesCacheCapacity,
		DisableBufferPool:      !enableBufferPool,
		DisableBlockCache:      !enableBlockCache,
		ErrorIfExist:           true,
		Compression:            opt.NoCompression,
	}
	if enableCompression {
		o.Compression = opt.DefaultCompression
	}

	db, err := leveldb.Open(tstor, o)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var (
		mu              = &sync.Mutex{}
		gGetStat        = &latencyStats{}
		gIterStat       = &latencyStats{}
		gWriteStat      = &latencyStats{}
		gTrasactionStat = &latencyStats{}
		startTime       = time.Now()

		writeReq    = make(chan *leveldb.Batch)
		writeAck    = make(chan error)
		writeAckAck = make(chan struct{})
	)

	go func() {
		for b := range writeReq {

			var err error
			if mrand.Float64() < transactionProb {
				log.Print("> Write using transaction")
				gTrasactionStat.start()
				var tr *leveldb.Transaction
				if tr, err = db.OpenTransaction(); err == nil {
					if err = tr.Write(b, nil); err == nil {
						if err = tr.Commit(); err == nil {
							gTrasactionStat.record(b.Len())
						}
					} else {
						tr.Discard()
					}
				}
			} else {
				gWriteStat.start()
				if err = db.Write(b, nil); err == nil {
					gWriteStat.record(b.Len())
				}
			}
			writeAck <- err
			<-writeAckAck
		}
	}()

	go func() {
		for {
			time.Sleep(3 * time.Second)

			log.Print("------------------------")

			log.Printf("> Elapsed=%v", time.Now().Sub(startTime))
			mu.Lock()
			log.Printf("> GetLatencyMin=%v GetLatencyMax=%v GetLatencyAvg=%v GetRatePerSec=%d",
				gGetStat.min, gGetStat.max, gGetStat.avg(), gGetStat.ratePerSec())
			log.Printf("> IterLatencyMin=%v IterLatencyMax=%v IterLatencyAvg=%v IterRatePerSec=%d",
				gIterStat.min, gIterStat.max, gIterStat.avg(), gIterStat.ratePerSec())
			log.Printf("> WriteLatencyMin=%v WriteLatencyMax=%v WriteLatencyAvg=%v WriteRatePerSec=%d",
				gWriteStat.min, gWriteStat.max, gWriteStat.avg(), gWriteStat.ratePerSec())
			log.Printf("> TransactionLatencyMin=%v TransactionLatencyMax=%v TransactionLatencyAvg=%v TransactionRatePerSec=%d",
				gTrasactionStat.min, gTrasactionStat.max, gTrasactionStat.avg(), gTrasactionStat.ratePerSec())
			mu.Unlock()

			cachedblock, _ := db.GetProperty("leveldb.cachedblock")
			openedtables, _ := db.GetProperty("leveldb.openedtables")
			alivesnaps, _ := db.GetProperty("leveldb.alivesnaps")
			aliveiters, _ := db.GetProperty("leveldb.aliveiters")
			blockpool, _ := db.GetProperty("leveldb.blockpool")
			log.Printf("> BlockCache=%s OpenedTables=%s AliveSnaps=%s AliveIter=%s BlockPool=%q",
				cachedblock, openedtables, alivesnaps, aliveiters, blockpool)

			log.Print("------------------------")
		}
	}()

	for ns, numKey := range numKeys {
		func(ns, numKey int) {
			log.Printf("[%02d] STARTING: numKey=%d", ns, numKey)

			keys := make([][]byte, numKey)
			for i := range keys {
				keys[i] = randomData(nil, byte(ns), 1, uint32(i), keyLen)
			}

			wg.Add(1)
			go func() {
				var wi uint32
				defer func() {
					log.Printf("[%02d] WRITER DONE #%d", ns, wi)
					wg.Done()
				}()

				var (
					b       = new(leveldb.Batch)
					k2, v2  []byte
					nReader int32
				)
				for atomic.LoadUint32(&done) == 0 {
					log.Printf("[%02d] WRITER #%d", ns, wi)

					b.Reset()
					for _, k1 := range keys {
						k2 = randomData(k2, byte(ns), 2, wi, keyLen)
						v2 = randomData(v2, byte(ns), 3, wi, valueLen)
						b.Put(k2, v2)
						b.Put(k1, k2)
					}
					writeReq <- b
					if err := <-writeAck; err != nil {
						writeAckAck <- struct{}{}
						fatalf(err, "[%02d] WRITER #%d db.Write: %v", ns, wi, err)
					}

					snap, err := db.GetSnapshot()
					if err != nil {
						writeAckAck <- struct{}{}
						fatalf(err, "[%02d] WRITER #%d db.GetSnapshot: %v", ns, wi, err)
					}

					writeAckAck <- struct{}{}

					wg.Add(1)
					atomic.AddInt32(&nReader, 1)
					go func(snapwi uint32, snap *leveldb.Snapshot) {
						var (
							ri       int
							iterStat = &latencyStats{}
							getStat  = &latencyStats{}
						)
						defer func() {
							mu.Lock()
							gGetStat.add(getStat)
							gIterStat.add(iterStat)
							mu.Unlock()

							atomic.AddInt32(&nReader, -1)
							log.Printf("[%02d] READER #%d.%d DONE Snap=%v Alive=%d IterLatency=%v GetLatency=%v", ns, snapwi, ri, snap, atomic.LoadInt32(&nReader), iterStat.avg(), getStat.avg())
							snap.Release()
							wg.Done()
						}()

						stopi := snapwi + 3
						for (ri < 3 || atomic.LoadUint32(&wi) < stopi) && atomic.LoadUint32(&done) == 0 {
							var n int
							iter := snap.NewIterator(dataPrefixSlice(byte(ns), 1), nil)
							iterStat.start()
							for iter.Next() {
								k1 := iter.Key()
								k2 := iter.Value()
								iterStat.record(1)

								if dataNS(k2) != byte(ns) {
									fatalf(nil, "[%02d] READER #%d.%d K%d invalid in-key NS: want=%d got=%d", ns, snapwi, ri, n, ns, dataNS(k2))
								}

								kwritei := dataI(k2)
								if kwritei != snapwi {
									fatalf(nil, "[%02d] READER #%d.%d K%d invalid in-key iter num: %d", ns, snapwi, ri, n, kwritei)
								}

								getStat.start()
								v2, err := snap.Get(k2, nil)
								if err != nil {
									fatalf(err, "[%02d] READER #%d.%d K%d snap.Get: %v\nk1: %x\n -> k2: %x", ns, snapwi, ri, n, err, k1, k2)
								}
								getStat.record(1)

								if checksum0, checksum1 := dataChecksum(v2); checksum0 != checksum1 {
									err := &errors.ErrCorrupted{Fd: storage.FileDesc{0xff, 0}, Err: fmt.Errorf("v2: %x: checksum mismatch: %v vs %v", v2, checksum0, checksum1)}
									fatalf(err, "[%02d] READER #%d.%d K%d snap.Get: %v\nk1: %x\n -> k2: %x", ns, snapwi, ri, n, err, k1, k2)
								}

								n++
								iterStat.start()
							}
							iter.Release()
							if err := iter.Error(); err != nil {
								fatalf(err, "[%02d] READER #%d.%d K%d iter.Error: %v", ns, snapwi, ri, numKey, err)
							}
							if n != numKey {
								fatalf(nil, "[%02d] READER #%d.%d missing keys: want=%d got=%d", ns, snapwi, ri, numKey, n)
							}

							ri++
						}
					}(wi, snap)

					atomic.AddUint32(&wi, 1)
				}
			}()

			delB := new(leveldb.Batch)
			wg.Add(1)
			go func() {
				var (
					i        int
					iterStat = &latencyStats{}
				)
				defer func() {
					log.Printf("[%02d] SCANNER DONE #%d", ns, i)
					wg.Done()
				}()

				time.Sleep(2 * time.Second)

				for atomic.LoadUint32(&done) == 0 {
					var n int
					delB.Reset()
					iter := db.NewIterator(dataNsSlice(byte(ns)), nil)
					iterStat.start()
					for iter.Next() && atomic.LoadUint32(&done) == 0 {
						k := iter.Key()
						v := iter.Value()
						iterStat.record(1)

						for ci, x := range [...][]byte{k, v} {
							checksum0, checksum1 := dataChecksum(x)
							if checksum0 != checksum1 {
								if ci == 0 {
									fatalf(nil, "[%02d] SCANNER %d.%d invalid key checksum: want %d, got %d\n%x -> %x", ns, i, n, checksum0, checksum1, k, v)
								} else {
									fatalf(nil, "[%02d] SCANNER %d.%d invalid value checksum: want %d, got %d\n%x -> %x", ns, i, n, checksum0, checksum1, k, v)
								}
							}
						}

						if dataPrefix(k) == 2 || mrand.Int()%999 == 0 {
							delB.Delete(k)
						}

						n++
						iterStat.start()
					}
					iter.Release()
					if err := iter.Error(); err != nil {
						fatalf(err, "[%02d] SCANNER #%d.%d iter.Error: %v", ns, i, n, err)
					}

					if n > 0 {
						log.Printf("[%02d] SCANNER #%d IterLatency=%v", ns, i, iterStat.avg())
					}

					if delB.Len() > 0 && atomic.LoadUint32(&done) == 0 {
						t := time.Now()
						writeReq <- delB
						if err := <-writeAck; err != nil {
							writeAckAck <- struct{}{}
							fatalf(err, "[%02d] SCANNER #%d db.Write: %v", ns, i, err)
						} else {
							writeAckAck <- struct{}{}
						}
						log.Printf("[%02d] SCANNER #%d Deleted=%d Time=%v", ns, i, delB.Len(), time.Now().Sub(t))
					}

					i++
				}
			}()
		}(ns, numKey)
	}

	go func() {
		sig := make(chan os.Signal)
		signal.Notify(sig, os.Interrupt, os.Kill)
		log.Printf("Got signal: %v, exiting...", <-sig)
		atomic.StoreUint32(&done, 1)
	}()

	wg.Wait()
}
