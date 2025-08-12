// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	lz4 "github.com/pierrec/lz4/v4"
	"google.golang.org/protobuf/proto"

	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/testutil"
)

var (
	c0ID = NewDeviceID([]byte{1})
	c1ID = NewDeviceID([]byte{2})
)

func TestPing(t *testing.T) {
	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := getRawConnection(NewConnection(c0ID, ar, bw, testutil.NoopCloser{}, newTestModel(), new(mockedConnectionInfo), CompressionAlways, testKeyGen))
	c0.Start()
	defer closeAndWait(c0, ar, bw)
	c1 := getRawConnection(NewConnection(c1ID, br, aw, testutil.NoopCloser{}, newTestModel(), new(mockedConnectionInfo), CompressionAlways, testKeyGen))
	c1.Start()
	defer closeAndWait(c1, ar, bw)
	c0.ClusterConfig(&ClusterConfig{}, nil)
	c1.ClusterConfig(&ClusterConfig{}, nil)

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

	c0 := getRawConnection(NewConnection(c0ID, ar, bw, testutil.NoopCloser{}, m0, new(mockedConnectionInfo), CompressionAlways, testKeyGen))
	c0.Start()
	defer closeAndWait(c0, ar, bw)
	c1 := NewConnection(c1ID, br, aw, testutil.NoopCloser{}, m1, new(mockedConnectionInfo), CompressionAlways, testKeyGen)
	c1.Start()
	defer closeAndWait(c1, ar, bw)
	c0.ClusterConfig(&ClusterConfig{}, nil)
	c1.ClusterConfig(&ClusterConfig{}, nil)

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

	c0.Index(ctx, &Index{Folder: "default"})
	c0.Index(ctx, &Index{Folder: "default"})

	if _, err := c0.Request(ctx, &Request{Folder: "default", Name: "foo"}); err == nil {
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

	rw := testutil.NewBlockingRW()
	c := getRawConnection(NewConnection(c0ID, rw, rw, testutil.NoopCloser{}, m, new(mockedConnectionInfo), CompressionAlways, testKeyGen))
	c.Start()
	defer closeAndWait(c, rw)

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		c.ClusterConfig(&ClusterConfig{}, nil)
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
	m0.indexFn = func(string, []FileInfo) {
		close(indexReceived)
		<-unblockIndex
	}
	m1 := newTestModel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := getRawConnection(NewConnection(c0ID, ar, bw, testutil.NoopCloser{}, m0, new(mockedConnectionInfo), CompressionNever, testKeyGen))
	c0.Start()
	defer closeAndWait(c0, ar, bw)
	c1 := NewConnection(c1ID, br, aw, testutil.NoopCloser{}, m1, new(mockedConnectionInfo), CompressionNever, testKeyGen)
	c1.Start()
	defer closeAndWait(c1, ar, bw)
	c0.ClusterConfig(&ClusterConfig{}, nil)
	c1.ClusterConfig(&ClusterConfig{}, nil)

	c1.Index(context.Background(), &Index{Folder: "default"})
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

	rw := testutil.NewBlockingRW()
	c := getRawConnection(NewConnection(c0ID, rw, &testutil.NoopRW{}, testutil.NoopCloser{}, m, new(mockedConnectionInfo), CompressionAlways, testKeyGen))
	c.Start()
	defer closeAndWait(c, rw)

	select {
	case c.outbox <- asyncMessage{&bep.Ping{}, nil}:
		t.Fatal("able to send ping before cluster config")
	case <-time.After(100 * time.Millisecond):
		// Allow some time for c.writerLoop to set up after c.Start
	}

	c.ClusterConfig(&ClusterConfig{}, nil)

	done := make(chan struct{})
	if ok := c.send(context.Background(), &bep.Ping{}, done); !ok {
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

	rw := testutil.NewBlockingRW()
	c := getRawConnection(NewConnection(c0ID, rw, rw, testutil.NoopCloser{}, m, new(mockedConnectionInfo), CompressionAlways, testKeyGen))
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

func TestUnmarshalFDPUv16v17(t *testing.T) {
	var fdpu bep.FileDownloadProgressUpdate

	m0, _ := hex.DecodeString("08cda1e2e3011278f3918787f3b89b8af2958887f0aa9389f3a08588f3aa8f96f39aa8a5f48b9188f19286a0f3848da4f3aba799f3beb489f0a285b9f487b684f2a3bda2f48598b4f2938a89f2a28badf187a0a2f2aebdbdf4849494f4808fbbf2b3a2adf2bb95bff0a6ada4f198ab9af29a9c8bf1abb793f3baabb2f188a6ba1a0020bb9390f60220f6d9e42220b0c7e2b2fdffffffff0120fdb2dfcdfbffffffff0120cedab1d50120bd8784c0feffffffff0120ace99591fdffffffff0120eed7d09af9ffffffff01")
	if err := proto.Unmarshal(m0, &fdpu); err != nil {
		t.Fatal("Unmarshalling message from v0.14.16:", err)
	}

	m1, _ := hex.DecodeString("0880f1969905128401f099b192f0abb1b9f3b280aff19e9aa2f3b89e84f484b39df1a7a6b0f1aea4b1f0adac94f3b39caaf1939281f1928a8af0abb1b0f0a8b3b3f3a88e94f2bd85acf29c97a9f2969da6f0b7a188f1908ea2f09a9c9bf19d86a6f29aada8f389bb95f0bf9d88f1a09d89f1b1a4b5f29b9eabf298a59df1b2a589f2979ebdf0b69880f18986b21a440a1508c7d8fb8897ca93d90910e8c4d8e8f2f8f0ccee010a1508afa8ffd8c085b393c50110e5bdedc3bddefe9b0b0a1408a1bedddba4cac5da3c10b8e5d9958ca7e3ec19225ae2f88cb2f8ffffffff018ceda99cfbffffffff01b9c298a407e295e8e9fcffffffff01f3b9ade5fcffffffff01c08bfea9fdffffffff01a2c2e5e1ffffffffff0186dcc5dafdffffffff01e9ffc7e507c9d89db8fdffffffff01")
	if err := proto.Unmarshal(m1, &fdpu); err != nil {
		t.Fatal("Unmarshalling message from v0.14.16:", err)
	}
}

func TestWriteCompressed(t *testing.T) {
	for _, random := range []bool{false, true} {
		buf := new(bytes.Buffer)
		c := &rawConnection{
			cr:          &countingReader{Reader: buf},
			cw:          &countingWriter{Writer: buf},
			compression: CompressionAlways,
		}

		msg := (&Response{Data: make([]byte, 10240)}).toWire()
		if random {
			// This should make the message incompressible.
			rand.Read(msg.Data)
		}

		if err := c.writeMessage(msg); err != nil {
			t.Fatal(err)
		}
		got, err := c.readMessage(make([]byte, 4))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.(*bep.Response).Data, msg.Data) {
			t.Error("received the wrong message")
		}

		hdr := &bep.Header{Type: typeOf(msg)}
		size := int64(2 + proto.Size(hdr) + 4 + proto.Size(msg))
		if c.cr.Tot() > size {
			t.Errorf("compression enlarged message from %d to %d",
				size, c.cr.Tot())
		}
	}
}

func TestLZ4Compression(t *testing.T) {
	for i := 0; i < 10; i++ {
		dataLen := 150 + rand.Intn(150)
		data := make([]byte, dataLen)
		_, err := io.ReadFull(rand.Reader, data[100:])
		if err != nil {
			t.Fatal(err)
		}

		comp := make([]byte, lz4.CompressBlockBound(dataLen))
		compLen, err := lz4Compress(data, comp)
		if err != nil {
			t.Errorf("compressing %d bytes: %v", dataLen, err)
			continue
		}

		res, err := lz4Decompress(comp[:compLen])
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

func TestLZ4CompressionUpdate(t *testing.T) {
	uncompressed := []byte("this is some arbitrary yet fairly compressible data")

	// Compressed, as created by the LZ4 implementation in Syncthing 1.18.6 and earlier.
	oldCompressed, _ := hex.DecodeString("00000033f0247468697320697320736f6d65206172626974726172792079657420666169726c7920636f6d707265737369626c652064617461")

	// Verify that we can decompress

	res, err := lz4Decompress(oldCompressed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(uncompressed, res) {
		t.Fatal("result does not match")
	}

	// Verify that our current compression is equivalent

	buf := make([]byte, 128)
	n, err := lz4Compress(uncompressed, buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(oldCompressed, buf[:n]) {
		t.Logf("%x", oldCompressed)
		t.Logf("%x", buf[:n])
		t.Fatal("compression does not match")
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

// TestClusterConfigAfterClose checks that ClusterConfig does not deadlock when
// ClusterConfig is called on a closed connection.
func TestClusterConfigAfterClose(t *testing.T) {
	m := newTestModel()

	rw := testutil.NewBlockingRW()
	c := getRawConnection(NewConnection(c0ID, rw, rw, testutil.NoopCloser{}, m, new(mockedConnectionInfo), CompressionAlways, testKeyGen))
	c.Start()
	defer closeAndWait(c, rw)

	c.internalClose(errManual)

	done := make(chan struct{})
	go func() {
		c.ClusterConfig(&ClusterConfig{}, nil)
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
	rw := testutil.NewBlockingRW()
	c := getRawConnection(NewConnection(c0ID, rw, &testutil.NoopRW{}, testutil.NoopCloser{}, m, new(mockedConnectionInfo), CompressionAlways, testKeyGen))
	m.ccFn = func(*ClusterConfig) {
		c.Close(errManual)
	}
	c.Start()
	defer closeAndWait(c, rw)

	c.inbox <- &bep.ClusterConfig{}

	select {
	case <-c.dispatcherLoopStopped:
	case <-time.After(time.Second):
		t.Fatal("timed out before dispatcher loop terminated")
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
