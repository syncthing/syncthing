package fs

//  type File interface {
//  	io.Closer
//  	io.Reader
//  	io.ReaderAt
//  	io.Seeker
//  	io.Writer // Write(p []byte) (n int, err error)
//  	io.WriterAt
//  	Name() string
//  	Truncate(size int64) error
//  	Stat() (FileInfo, error)
//  	Sync() error
//  }

// basicFile implements the fs.File interface on top of an os.File
type ofuseFile struct {
	basicFile
	fs         *OwnFuseFilesystem
	wasChanged bool
}

//func (of ofuseFile) calculateHash() error {
//	blocksResult, err = scanner.Blocks(ctx, r, protocol.MinBlockSize, int64(len(bs)), nil, useWeakHash)
//	if err != nil {
//		return 0 // Context done
//	}
//}

func (of ofuseFile) Close() error {
	l.Warnf("================> Syncthing closes %s", of.name)
	l.Warnf("==================> Syncthing changed %s", of.name)

	return of.basicFile.Close()
}

func (of ofuseFile) Write(p []byte) (n int, err error) {
	of.wasChanged = true
	return of.basicFile.Write(p)
}

func (of ofuseFile) WriteAt(p []byte, off int64) (n int, err error) {
	of.wasChanged = true
	return of.basicFile.WriteAt(p, off)
}

func (of ofuseFile) Truncate(size int64) error {
	of.wasChanged = true
	return of.basicFile.Truncate(size)
}
