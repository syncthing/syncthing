// +build darwin

package protocol

// Darwin uses NFD normalization

import "code.google.com/p/go.text/unicode/norm"

type nativeModel struct {
	next Model
}

func (m nativeModel) Index(nodeID string, files []FileInfo) {
	for i := range files {
		files[i].Name = norm.NFD.String(files[i].Name)
	}
	m.next.Index(nodeID, files)
}

func (m nativeModel) IndexUpdate(nodeID string, files []FileInfo) {
	for i := range files {
		files[i].Name = norm.NFD.String(files[i].Name)
	}
	m.next.IndexUpdate(nodeID, files)
}

func (m nativeModel) Request(nodeID, repo string, name string, offset int64, size int) ([]byte, error) {
	name = norm.NFD.String(name)
	return m.next.Request(nodeID, repo, name, offset, size)
}

func (m nativeModel) Close(nodeID string, err error) {
	m.next.Close(nodeID, err)
}
