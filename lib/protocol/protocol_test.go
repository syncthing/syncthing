// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"runtime"
	"sync"
	"testing"
	"testing/quick"
	"time"

	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/testutils"
)

var (
	c0ID     = NewDeviceID([]byte{1})
	c1ID     = NewDeviceID([]byte{2})
	quickCfg = &quick.Config{}
)

func TestPing(t *testing.T) {
	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := getRawConnection(NewConnection(c0ID, ar, bw, testutils.NoopCloser{}, newTestModel(), new(mockedConnectionInfo), CompressionAlways, nil))
	c0.Start()
	defer closeAndWait(c0, ar, bw)
	c1 := getRawConnection(NewConnection(c1ID, br, aw, testutils.NoopCloser{}, newTestModel(), new(mockedConnectionInfo), CompressionAlways, nil))
	c1.Start()
	defer closeAndWait(c1, ar, bw)
	c0.ClusterConfig(ClusterConfig{})
	c1.ClusterConfig(ClusterConfig{})

	if ok := c0.ping(); !ok {
		t.Error("c0 ping failed")
	}
	if ok := c1.ping(); !ok {
		t.Error("c1 ping failed")
	}
}

var errManual = errors.New("manual close")

func TestClose(t *testing.T) {
	m0 := newTestModel()
	m1 := newTestModel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := getRawConnection(NewConnection(c0ID, ar, bw, testutils.NoopCloser{}, m0, new(mockedConnectionInfo), CompressionAlways, nil))
	c0.Start()
	defer closeAndWait(c0, ar, bw)
	c1 := NewConnection(c1ID, br, aw, testutils.NoopCloser{}, m1, new(mockedConnectionInfo), CompressionAlways, nil)
	c1.Start()
	defer closeAndWait(c1, ar, bw)
	c0.ClusterConfig(ClusterConfig{})
	c1.ClusterConfig(ClusterConfig{})

	c0.internalClose(errManual)

	<-c0.closed
	if err := m0.closedError(); err != errManual {
		t.Fatal("Connection should be closed")
	}

	// None of these should panic, some should return an error

	if c0.ping() {
		t.Error("Ping should not return true")
	}

	ctx := context.Background()

	c0.Index(ctx, "default", nil)
	c0.Index(ctx, "default", nil)

	if _, err := c0.Request(ctx, "default", "foo", 0, 0, 0, nil, 0, false); err == nil {
		t.Error("Request should return an error")
	}
}

// TestCloseOnBlockingSend checks that the connection does not deadlock when
// Close is called while the underlying connection is broken (send blocks).
// https://github.com/syncthing/syncthing/pull/5442
func TestCloseOnBlockingSend(t *testing.T) {
	oldCloseTimeout := CloseTimeout
	CloseTimeout = 100 * time.Millisecond
	defer func() {
		CloseTimeout = oldCloseTimeout
	}()

	m := newTestModel()

	rw := testutils.NewBlockingRW()
	c := getRawConnection(NewConnection(c0ID, rw, rw, testutils.NoopCloser{}, m, new(mockedConnectionInfo), CompressionAlways, nil))
	c.Start()
	defer closeAndWait(c, rw)

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		c.ClusterConfig(ClusterConfig{})
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		c.Close(errManual)
		wg.Done()
	}()

	// This simulates an error from ping timeout
	wg.Add(1)
	go func() {
		c.internalClose(ErrTimeout)
		wg.Done()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out before all functions returned")
	}
}

func TestCloseRace(t *testing.T) {
	indexReceived := make(chan struct{})
	unblockIndex := make(chan struct{})
	m0 := newTestModel()
	m0.indexFn = func(_ DeviceID, _ string, _ []FileInfo) {
		close(indexReceived)
		<-unblockIndex
	}
	m1 := newTestModel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := getRawConnection(NewConnection(c0ID, ar, bw, testutils.NoopCloser{}, m0, new(mockedConnectionInfo), CompressionNever, nil))
	c0.Start()
	defer closeAndWait(c0, ar, bw)
	c1 := NewConnection(c1ID, br, aw, testutils.NoopCloser{}, m1, new(mockedConnectionInfo), CompressionNever, nil)
	c1.Start()
	defer closeAndWait(c1, ar, bw)
	c0.ClusterConfig(ClusterConfig{})
	c1.ClusterConfig(ClusterConfig{})

	c1.Index(context.Background(), "default", nil)
	select {
	case <-indexReceived:
	case <-time.After(time.Second):
		t.Fatal("timed out before receiving index")
	}

	go c0.internalClose(errManual)
	select {
	case <-c0.closed:
	case <-time.After(time.Second):
		t.Fatal("timed out before c0.closed was closed")
	}

	select {
	case <-m0.closedCh:
		t.Errorf("receiver.Closed called before receiver.Index")
	default:
	}

	close(unblockIndex)

	if err := m0.closedError(); err != errManual {
		t.Fatal("Connection should be closed")
	}
}

func TestClusterConfigFirst(t *testing.T) {
	m := newTestModel()

	rw := testutils.NewBlockingRW()
	c := getRawConnection(NewConnection(c0ID, rw, &testutils.NoopRW{}, testutils.NoopCloser{}, m, new(mockedConnectionInfo), CompressionAlways, nil))
	c.Start()
	defer closeAndWait(c, rw)

	select {
	case c.outbox <- asyncMessage{&Ping{}, nil}:
		t.Fatal("able to send ping before cluster config")
	case <-time.After(100 * time.Millisecond):
		// Allow some time for c.writerLoop to setup after c.Start
	}

	c.ClusterConfig(ClusterConfig{})

	done := make(chan struct{})
	if ok := c.send(context.Background(), &Ping{}, done); !ok {
		t.Fatal("send ping after cluster config returned false")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out before ping was sent")
	}

	done = make(chan struct{})
	go func() {
		c.internalClose(errManual)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close didn't return before timeout")
	}

	if err := m.closedError(); err != errManual {
		t.Fatal("Connection should be closed")
	}
}

// TestCloseTimeout checks that calling Close times out and proceeds, if sending
// the close message does not succeed.
func TestCloseTimeout(t *testing.T) {
	oldCloseTimeout := CloseTimeout
	CloseTimeout = 100 * time.Millisecond
	defer func() {
		CloseTimeout = oldCloseTimeout
	}()

	m := newTestModel()

	rw := testutils.NewBlockingRW()
	c := getRawConnection(NewConnection(c0ID, rw, rw, testutils.NoopCloser{}, m, new(mockedConnectionInfo), CompressionAlways, nil))
	c.Start()
	defer closeAndWait(c, rw)

	done := make(chan struct{})
	go func() {
		c.Close(errManual)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * CloseTimeout):
		t.Fatal("timed out before Close returned")
	}
}

func TestMarshalIndexMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 Index) bool {
		if len(m1.Files) == 0 {
			m1.Files = nil
		}
		for i, f := range m1.Files {
			if len(f.BlocksHash) == 0 {
				m1.Files[i].BlocksHash = nil
			}
			if len(f.VersionHash) == 0 {
				m1.Files[i].VersionHash = nil
			}
			if len(f.Blocks) == 0 {
				m1.Files[i].Blocks = nil
			} else {
				for j := range f.Blocks {
					f.Blocks[j].Offset = 0
					if len(f.Blocks[j].Hash) == 0 {
						f.Blocks[j].Hash = nil
					}
				}
			}
			if len(f.Version.Counters) == 0 {
				m1.Files[i].Version.Counters = nil
			}
			if len(f.Encrypted) == 0 {
				m1.Files[i].Encrypted = nil
			}
		}

		return testMarshal(t, "index", &m1, &Index{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalRequestMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 Request) bool {
		if len(m1.Hash) == 0 {
			m1.Hash = nil
		}
		return testMarshal(t, "request", &m1, &Request{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalResponseMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 Response) bool {
		if len(m1.Data) == 0 {
			m1.Data = nil
		}
		return testMarshal(t, "response", &m1, &Response{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalClusterConfigMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 ClusterConfig) bool {
		if len(m1.Folders) == 0 {
			m1.Folders = nil
		}
		for i := range m1.Folders {
			if len(m1.Folders[i].Devices) == 0 {
				m1.Folders[i].Devices = nil
			}
			for j := range m1.Folders[i].Devices {
				if len(m1.Folders[i].Devices[j].Addresses) == 0 {
					m1.Folders[i].Devices[j].Addresses = nil
				}
				if len(m1.Folders[i].Devices[j].EncryptionPasswordToken) == 0 {
					m1.Folders[i].Devices[j].EncryptionPasswordToken = nil
				}
			}
		}

		return testMarshal(t, "clusterconfig", &m1, &ClusterConfig{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalCloseMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 Close) bool {
		return testMarshal(t, "close", &m1, &Close{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalFDPU(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 FileDownloadProgressUpdate) bool {
		if len(m1.Version.Counters) == 0 {
			m1.Version.Counters = nil
		}
		if len(m1.BlockIndexes) == 0 {
			m1.BlockIndexes = nil
		}
		return testMarshal(t, "fdpu", &m1, &FileDownloadProgressUpdate{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestUnmarshalFDPUv16v17(t *testing.T) {
	var fdpu FileDownloadProgressUpdate

	m0, _ := hex.DecodeString("08cda1e2e3011278f3918787f3b89b8af2958887f0aa9389f3a08588f3aa8f96f39aa8a5f48b9188f19286a0f3848da4f3aba799f3beb489f0a285b9f487b684f2a3bda2f48598b4f2938a89f2a28badf187a0a2f2aebdbdf4849494f4808fbbf2b3a2adf2bb95bff0a6ada4f198ab9af29a9c8bf1abb793f3baabb2f188a6ba1a0020bb9390f60220f6d9e42220b0c7e2b2fdffffffff0120fdb2dfcdfbffffffff0120cedab1d50120bd8784c0feffffffff0120ace99591fdffffffff0120eed7d09af9ffffffff01")
	if err := fdpu.Unmarshal(m0); err != nil {
		t.Fatal("Unmarshalling message from v0.14.16:", err)
	}

	m1, _ := hex.DecodeString("0880f1969905128401f099b192f0abb1b9f3b280aff19e9aa2f3b89e84f484b39df1a7a6b0f1aea4b1f0adac94f3b39caaf1939281f1928a8af0abb1b0f0a8b3b3f3a88e94f2bd85acf29c97a9f2969da6f0b7a188f1908ea2f09a9c9bf19d86a6f29aada8f389bb95f0bf9d88f1a09d89f1b1a4b5f29b9eabf298a59df1b2a589f2979ebdf0b69880f18986b21a440a1508c7d8fb8897ca93d90910e8c4d8e8f2f8f0ccee010a1508afa8ffd8c085b393c50110e5bdedc3bddefe9b0b0a1408a1bedddba4cac5da3c10b8e5d9958ca7e3ec19225ae2f88cb2f8ffffffff018ceda99cfbffffffff01b9c298a407e295e8e9fcffffffff01f3b9ade5fcffffffff01c08bfea9fdffffffff01a2c2e5e1ffffffffff0186dcc5dafdffffffff01e9ffc7e507c9d89db8fdffffffff01")
	if err := fdpu.Unmarshal(m1); err != nil {
		t.Fatal("Unmarshalling message from v0.14.16:", err)
	}
}

func testMarshal(t *testing.T, prefix string, m1, m2 message) bool {
	buf, err := m1.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	err = m2.Unmarshal(buf)
	if err != nil {
		t.Fatal(err)
	}

	bs1, _ := json.MarshalIndent(m1, "", "  ")
	bs2, _ := json.MarshalIndent(m2, "", "  ")
	if !bytes.Equal(bs1, bs2) {
		ioutil.WriteFile(prefix+"-1.txt", bs1, 0644)
		ioutil.WriteFile(prefix+"-2.txt", bs2, 0644)
		return false
	}

	return true
}

func TestLZ4Compression(t *testing.T) {
	for i := 0; i < 10; i++ {
		dataLen := 150 + rand.Intn(150)
		data := make([]byte, dataLen)
		_, err := io.ReadFull(rand.Reader, data[100:])
		if err != nil {
			t.Fatal(err)
		}
		comp, err := lz4Compress(data)
		if err != nil {
			t.Errorf("compressing %d bytes: %v", dataLen, err)
			continue
		}

		res, err := lz4Decompress(comp)
		if err != nil {
			t.Errorf("decompressing %d bytes to %d: %v", len(comp), dataLen, err)
			continue
		}
		if len(res) != len(data) {
			t.Errorf("Incorrect len %d != expected %d", len(res), len(data))
		}
		if !bytes.Equal(data, res) {
			t.Error("Incorrect decompressed data")
		}
		t.Logf("OK #%d, %d -> %d -> %d", i, dataLen, len(comp), dataLen)
	}
}

func TestStressLZ4CompressGrows(t *testing.T) {
	success := 0
	for i := 0; i < 100; i++ {
		// Create a slize that is precisely one min block size, fill it with
		// random data. This shouldn't compress at all, so will in fact
		// become larger when LZ4 does its thing.
		data := make([]byte, MinBlockSize)
		if _, err := rand.Reader.Read(data); err != nil {
			t.Fatal("randomness failure")
		}

		comp, err := lz4Compress(data)
		if err != nil {
			t.Fatal("unexpected compression error: ", err)
		}
		if len(comp) < len(data) {
			// data size should grow. We must have been really unlucky in
			// the random generation, try again.
			continue
		}

		// Putting it into the buffer pool shouldn't panic because the block
		// should come from there to begin with.
		BufferPool.Put(comp)
		success++
	}
	if success == 0 {
		t.Fatal("unable to find data that grows when compressed")
	}
}

func TestCheckFilename(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		// Valid filenames
		{"foo", true},
		{"foo/bar/baz", true},
		{"foo/bar:baz", true}, // colon is ok in general, will be filtered on windows
		{`\`, true},           // path separator on the wire is forward slash, so as above
		{`\.`, true},
		{`\..`, true},
		{".foo", true},
		{"foo..", true},

		// Invalid filenames
		{"foo/..", false},
		{"foo/../bar", false},
		{"../foo/../bar", false},
		{"", false},
		{".", false},
		{"..", false},
		{"/", false},
		{"/.", false},
		{"/..", false},
		{"/foo", false},
		{"./foo", false},
		{"foo./", false},
		{"foo/.", false},
		{"foo/", false},
	}

	for _, tc := range cases {
		err := checkFilename(tc.name)
		if (err == nil) != tc.ok {
			t.Errorf("Unexpected result for checkFilename(%q): %v", tc.name, err)
		}
	}
}

func TestCheckConsistency(t *testing.T) {
	cases := []struct {
		fi FileInfo
		ok bool
	}{
		{
			// valid
			fi: FileInfo{
				Name:   "foo",
				Type:   FileInfoTypeFile,
				Blocks: []BlockInfo{{Size: 1234, Offset: 0, Hash: []byte{1, 2, 3, 4}}},
			},
			ok: true,
		},
		{
			// deleted with blocks
			fi: FileInfo{
				Name:    "foo",
				Deleted: true,
				Type:    FileInfoTypeFile,
				Blocks:  []BlockInfo{{Size: 1234, Offset: 0, Hash: []byte{1, 2, 3, 4}}},
			},
			ok: false,
		},
		{
			// no blocks
			fi: FileInfo{
				Name: "foo",
				Type: FileInfoTypeFile,
			},
			ok: false,
		},
		{
			// directory with blocks
			fi: FileInfo{
				Name:   "foo",
				Type:   FileInfoTypeDirectory,
				Blocks: []BlockInfo{{Size: 1234, Offset: 0, Hash: []byte{1, 2, 3, 4}}},
			},
			ok: false,
		},
	}

	for _, tc := range cases {
		err := checkFileInfoConsistency(tc.fi)
		if tc.ok && err != nil {
			t.Errorf("Unexpected error %v (want nil) for %v", err, tc.fi)
		}
		if !tc.ok && err == nil {
			t.Errorf("Unexpected nil error for %v", tc.fi)
		}
	}
}

func TestBlockSize(t *testing.T) {
	cases := []struct {
		fileSize  int64
		blockSize int
	}{
		{1 << KiB, 128 << KiB},
		{1 << MiB, 128 << KiB},
		{499 << MiB, 256 << KiB},
		{500 << MiB, 512 << KiB},
		{501 << MiB, 512 << KiB},
		{1 << GiB, 1 << MiB},
		{2 << GiB, 2 << MiB},
		{3 << GiB, 2 << MiB},
		{500 << GiB, 16 << MiB},
		{50000 << GiB, 16 << MiB},
	}

	for _, tc := range cases {
		size := BlockSize(tc.fileSize)
		if size != tc.blockSize {
			t.Errorf("BlockSize(%d), size=%d, expected %d", tc.fileSize, size, tc.blockSize)
		}
	}
}

var blockSize int

func BenchmarkBlockSize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		blockSize = BlockSize(16 << 30)
	}
}

func TestLocalFlagBits(t *testing.T) {
	var f FileInfo
	if f.IsIgnored() || f.MustRescan() || f.IsInvalid() {
		t.Error("file should have no weird bits set by default")
	}

	f.SetIgnored()
	if !f.IsIgnored() || f.MustRescan() || !f.IsInvalid() {
		t.Error("file should be ignored and invalid")
	}

	f.SetMustRescan()
	if f.IsIgnored() || !f.MustRescan() || !f.IsInvalid() {
		t.Error("file should be must-rescan and invalid")
	}

	f.SetUnsupported()
	if f.IsIgnored() || f.MustRescan() || !f.IsInvalid() {
		t.Error("file should be invalid")
	}
}

func TestIsEquivalent(t *testing.T) {
	b := func(v bool) *bool {
		return &v
	}

	type testCase struct {
		a         FileInfo
		b         FileInfo
		ignPerms  *bool // nil means should not matter, we'll test both variants
		ignBlocks *bool
		ignFlags  uint32
		eq        bool
	}
	cases := []testCase{
		// Empty FileInfos are equivalent
		{eq: true},

		// Various basic attributes, all of which cause ineqality when
		// they differ
		{
			a:  FileInfo{Name: "foo"},
			b:  FileInfo{Name: "bar"},
			eq: false,
		},
		{
			a:  FileInfo{Type: FileInfoTypeFile},
			b:  FileInfo{Type: FileInfoTypeDirectory},
			eq: false,
		},
		{
			a:  FileInfo{Size: 1234},
			b:  FileInfo{Size: 2345},
			eq: false,
		},
		{
			a:  FileInfo{Deleted: false},
			b:  FileInfo{Deleted: true},
			eq: false,
		},
		{
			a:  FileInfo{RawInvalid: false},
			b:  FileInfo{RawInvalid: true},
			eq: false,
		},
		{
			a:  FileInfo{ModifiedS: 1234},
			b:  FileInfo{ModifiedS: 2345},
			eq: false,
		},
		{
			a:  FileInfo{ModifiedNs: 1234},
			b:  FileInfo{ModifiedNs: 2345},
			eq: false,
		},

		// Special handling of local flags and invalidity. "MustRescan"
		// files are never equivalent to each other. Otherwise, equivalence
		// is based just on whether the file becomes IsInvalid() or not, not
		// the specific reason or flag bits.
		{
			a:  FileInfo{LocalFlags: FlagLocalMustRescan},
			b:  FileInfo{LocalFlags: FlagLocalMustRescan},
			eq: false,
		},
		{
			a:  FileInfo{RawInvalid: true},
			b:  FileInfo{RawInvalid: true},
			eq: true,
		},
		{
			a:  FileInfo{LocalFlags: FlagLocalUnsupported},
			b:  FileInfo{LocalFlags: FlagLocalUnsupported},
			eq: true,
		},
		{
			a:  FileInfo{RawInvalid: true},
			b:  FileInfo{LocalFlags: FlagLocalUnsupported},
			eq: true,
		},
		{
			a:  FileInfo{LocalFlags: 0},
			b:  FileInfo{LocalFlags: FlagLocalReceiveOnly},
			eq: false,
		},
		{
			a:        FileInfo{LocalFlags: 0},
			b:        FileInfo{LocalFlags: FlagLocalReceiveOnly},
			ignFlags: FlagLocalReceiveOnly,
			eq:       true,
		},

		// Difference in blocks is not OK
		{
			a:         FileInfo{Blocks: []BlockInfo{{Hash: []byte{1, 2, 3, 4}}}},
			b:         FileInfo{Blocks: []BlockInfo{{Hash: []byte{2, 3, 4, 5}}}},
			ignBlocks: b(false),
			eq:        false,
		},

		// ... unless we say it is
		{
			a:         FileInfo{Blocks: []BlockInfo{{Hash: []byte{1, 2, 3, 4}}}},
			b:         FileInfo{Blocks: []BlockInfo{{Hash: []byte{2, 3, 4, 5}}}},
			ignBlocks: b(true),
			eq:        true,
		},

		// Difference in permissions is not OK.
		{
			a:        FileInfo{Permissions: 0444},
			b:        FileInfo{Permissions: 0666},
			ignPerms: b(false),
			eq:       false,
		},

		// ... unless we say it is
		{
			a:        FileInfo{Permissions: 0666},
			b:        FileInfo{Permissions: 0444},
			ignPerms: b(true),
			eq:       true,
		},

		// These attributes are not checked at all
		{
			a:  FileInfo{NoPermissions: false},
			b:  FileInfo{NoPermissions: true},
			eq: true,
		},
		{
			a:  FileInfo{Version: Vector{Counters: []Counter{{ID: 1, Value: 42}}}},
			b:  FileInfo{Version: Vector{Counters: []Counter{{ID: 42, Value: 1}}}},
			eq: true,
		},
		{
			a:  FileInfo{Sequence: 1},
			b:  FileInfo{Sequence: 2},
			eq: true,
		},

		// The block size is not checked (but this would fail the blocks
		// check in real world)
		{
			a:  FileInfo{RawBlockSize: 1},
			b:  FileInfo{RawBlockSize: 2},
			eq: true,
		},

		// The symlink target is checked for symlinks
		{
			a:  FileInfo{Type: FileInfoTypeSymlink, SymlinkTarget: "a"},
			b:  FileInfo{Type: FileInfoTypeSymlink, SymlinkTarget: "b"},
			eq: false,
		},

		// ... but not for non-symlinks
		{
			a:  FileInfo{Type: FileInfoTypeFile, SymlinkTarget: "a"},
			b:  FileInfo{Type: FileInfoTypeFile, SymlinkTarget: "b"},
			eq: true,
		},
	}

	if runtime.GOOS == "windows" {
		// On windows we only check the user writable bit of the permission
		// set, so these are equivalent.
		cases = append(cases, testCase{
			a:        FileInfo{Permissions: 0777},
			b:        FileInfo{Permissions: 0600},
			ignPerms: b(false),
			eq:       true,
		})
	}

	for i, tc := range cases {
		// Check the standard attributes with all permutations of the
		// special ignore flags, unless the value of those flags are given
		// in the tests.
		for _, ignPerms := range []bool{true, false} {
			for _, ignBlocks := range []bool{true, false} {
				if tc.ignPerms != nil && *tc.ignPerms != ignPerms {
					continue
				}
				if tc.ignBlocks != nil && *tc.ignBlocks != ignBlocks {
					continue
				}

				if res := tc.a.isEquivalent(tc.b, 0, ignPerms, ignBlocks, tc.ignFlags); res != tc.eq {
					t.Errorf("Case %d:\na: %v\nb: %v\na.IsEquivalent(b, %v, %v) => %v, expected %v", i, tc.a, tc.b, ignPerms, ignBlocks, res, tc.eq)
				}
				if res := tc.b.isEquivalent(tc.a, 0, ignPerms, ignBlocks, tc.ignFlags); res != tc.eq {
					t.Errorf("Case %d:\na: %v\nb: %v\nb.IsEquivalent(a, %v, %v) => %v, expected %v", i, tc.a, tc.b, ignPerms, ignBlocks, res, tc.eq)
				}
			}
		}
	}
}

func TestSha256OfEmptyBlock(t *testing.T) {
	// every block size should have a correct entry in sha256OfEmptyBlock
	for blockSize := MinBlockSize; blockSize <= MaxBlockSize; blockSize *= 2 {
		expected := sha256.Sum256(make([]byte, blockSize))
		if sha256OfEmptyBlock[blockSize] != expected {
			t.Error("missing or wrong hash for block of size", blockSize)
		}
	}
}

// TestClusterConfigAfterClose checks that ClusterConfig does not deadlock when
// ClusterConfig is called on a closed connection.
func TestClusterConfigAfterClose(t *testing.T) {
	m := newTestModel()

	rw := testutils.NewBlockingRW()
	c := getRawConnection(NewConnection(c0ID, rw, rw, testutils.NoopCloser{}, m, new(mockedConnectionInfo), CompressionAlways, nil))
	c.Start()
	defer closeAndWait(c, rw)

	c.internalClose(errManual)

	done := make(chan struct{})
	go func() {
		c.ClusterConfig(ClusterConfig{})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out before Cluster Config returned")
	}
}

func TestDispatcherToCloseDeadlock(t *testing.T) {
	// Verify that we don't deadlock when calling Close() from within one of
	// the model callbacks (ClusterConfig).
	m := newTestModel()
	rw := testutils.NewBlockingRW()
	c := getRawConnection(NewConnection(c0ID, rw, &testutils.NoopRW{}, testutils.NoopCloser{}, m, new(mockedConnectionInfo), CompressionAlways, nil))
	m.ccFn = func(devID DeviceID, cc ClusterConfig) {
		c.Close(errManual)
	}
	c.Start()
	defer closeAndWait(c, rw)

	c.inbox <- &ClusterConfig{}

	select {
	case <-c.dispatcherLoopStopped:
	case <-time.After(time.Second):
		t.Fatal("timed out before dispatcher loop terminated")
	}
}

func TestBlocksEqual(t *testing.T) {
	blocksOne := []BlockInfo{{Hash: []byte{1, 2, 3, 4}}}
	blocksTwo := []BlockInfo{{Hash: []byte{5, 6, 7, 8}}}
	hashOne := []byte{42, 42, 42, 42}
	hashTwo := []byte{29, 29, 29, 29}

	cases := []struct {
		b1 []BlockInfo
		h1 []byte
		b2 []BlockInfo
		h2 []byte
		eq bool
	}{
		{blocksOne, hashOne, blocksOne, hashOne, true},  // everything equal
		{blocksOne, hashOne, blocksTwo, hashTwo, false}, // nothing equal
		{blocksOne, hashOne, blocksOne, nil, true},      // blocks compared
		{blocksOne, nil, blocksOne, nil, true},          // blocks compared
		{blocksOne, nil, blocksTwo, nil, false},         // blocks compared
		{blocksOne, hashOne, blocksTwo, hashOne, true},  // hashes equal, blocks not looked at
		{blocksOne, hashOne, blocksOne, hashTwo, true},  // hashes different, blocks compared
		{blocksOne, hashOne, blocksTwo, hashTwo, false}, // hashes different, blocks compared
		{blocksOne, hashOne, nil, nil, false},           // blocks is different from no blocks
		{blocksOne, nil, nil, nil, false},               // blocks is different from no blocks
		{nil, hashOne, nil, nil, true},                  // nil blocks are equal, even of one side has a hash
	}

	for _, tc := range cases {
		f1 := FileInfo{Blocks: tc.b1, BlocksHash: tc.h1}
		f2 := FileInfo{Blocks: tc.b2, BlocksHash: tc.h2}

		if !f1.BlocksEqual(f1) {
			t.Error("f1 is always equal to itself", f1)
		}
		if !f2.BlocksEqual(f2) {
			t.Error("f2 is always equal to itself", f2)
		}
		if res := f1.BlocksEqual(f2); res != tc.eq {
			t.Log("f1", f1.BlocksHash, f1.Blocks)
			t.Log("f2", f2.BlocksHash, f2.Blocks)
			t.Errorf("f1.BlocksEqual(f2) == %v but should be %v", res, tc.eq)
		}
		if res := f2.BlocksEqual(f1); res != tc.eq {
			t.Log("f1", f1.BlocksHash, f1.Blocks)
			t.Log("f2", f2.BlocksHash, f2.Blocks)
			t.Errorf("f2.BlocksEqual(f1) == %v but should be %v", res, tc.eq)
		}
	}
}

func TestIndexIDString(t *testing.T) {
	// Index ID is a 64 bit, zero padded hex integer.
	var i IndexID = 42
	if i.String() != "0x000000000000002A" {
		t.Error(i.String())
	}
}

func closeAndWait(c interface{}, closers ...io.Closer) {
	for _, closer := range closers {
		closer.Close()
	}
	var raw *rawConnection
	switch i := c.(type) {
	case *rawConnection:
		raw = i
	default:
		raw = getRawConnection(c.(Connection))
	}
	raw.internalClose(ErrClosed)
	raw.loopWG.Wait()
}

func getRawConnection(c Connection) *rawConnection {
	var raw *rawConnection
	switch i := c.(type) {
	case wireFormatConnection:
		raw = i.Connection.(encryptedConnection).conn
	case encryptedConnection:
		raw = i.conn
	}
	return raw
}
