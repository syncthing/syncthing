// +build !windows,!darwin

package protocol

// Normal Unixes uses NFC and slashes, which is the wire format.

type nativeModel struct {
	next Model
}

func (m nativeModel) Index(nodeID string, files []FileInfo) {
	m.next.Index(nodeID, files)
}

func (m nativeModel) IndexUpdate(nodeID string, files []FileInfo) {
	m.next.IndexUpdate(nodeID, files)
}

func (m nativeModel) Request(nodeID, repo string, name string, offset int64, size int) ([]byte, error) {
	return m.next.Request(nodeID, repo, name, offset, size)
}

func (m nativeModel) Close(nodeID string, err error) {
	m.next.Close(nodeID, err)
}
