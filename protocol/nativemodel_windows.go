// +build windows

package protocol

// Windows uses backslashes as file separator

import "path/filepath"

type nativeModel struct {
	next Model
}

func (m nativeModel) Index(nodeID string, repo string, files []FileInfo) {
	for i := range files {
		files[i].Name = filepath.FromSlash(files[i].Name)
	}
	m.next.Index(nodeID, repo, files)
}

func (m nativeModel) IndexUpdate(nodeID string, repo string, files []FileInfo) {
	for i := range files {
		files[i].Name = filepath.FromSlash(files[i].Name)
	}
	m.next.IndexUpdate(nodeID, repo, files)
}

func (m nativeModel) Request(nodeID, repo string, name string, offset int64, size int) ([]byte, error) {
	name = filepath.FromSlash(name)
	return m.next.Request(nodeID, repo, name, offset, size)
}

func (m nativeModel) ClusterConfig(nodeID string, config ClusterConfigMessage) {
	m.next.ClusterConfig(nodeID, config)
}

func (m nativeModel) Close(nodeID string, err error) {
	m.next.Close(nodeID, err)
}
