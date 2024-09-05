package blockstorage

type HashBlockStorageI interface {
	Get(hash []byte) (data []byte, ok bool)
	Set(hash []byte, data []byte)
	Delete(hash []byte)
}
