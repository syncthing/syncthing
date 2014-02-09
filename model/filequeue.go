package model

import (
	"log"
	"sort"
	"sync"
	"time"
)

type Monitor interface {
	FileBegins(<-chan content) error
	FileDone() error
}

type FileQueue struct {
	files        queuedFileList
	sorted       bool
	fmut         sync.Mutex // protects files and sorted
	availability map[string][]string
	amut         sync.Mutex // protects availability
	queued       map[string]bool
}

type queuedFile struct {
	name         string
	blocks       []Block
	activeBlocks []bool
	given        int
	remaining    int
	channel      chan content
	nodes        []string
	nodesChecked time.Time
	monitor      Monitor
}

type content struct {
	offset int64
	data   []byte
}

type queuedFileList []queuedFile

func (l queuedFileList) Len() int { return len(l) }

func (l queuedFileList) Swap(a, b int) { l[a], l[b] = l[b], l[a] }

func (l queuedFileList) Less(a, b int) bool {
	// Sort by most blocks already given out, then alphabetically
	if l[a].given != l[b].given {
		return l[a].given > l[b].given
	}
	return l[a].name < l[b].name
}

type queuedBlock struct {
	name  string
	block Block
	index int
}

func NewFileQueue() *FileQueue {
	return &FileQueue{
		availability: make(map[string][]string),
		queued:       make(map[string]bool),
	}
}

func (q *FileQueue) Add(name string, blocks []Block, monitor Monitor) {
	q.fmut.Lock()
	defer q.fmut.Unlock()

	if q.queued[name] {
		return
	}

	q.files = append(q.files, queuedFile{
		name:         name,
		blocks:       blocks,
		activeBlocks: make([]bool, len(blocks)),
		remaining:    len(blocks),
		channel:      make(chan content),
		monitor:      monitor,
	})
	q.queued[name] = true
	q.sorted = false
}

func (q *FileQueue) Len() int {
	q.fmut.Lock()
	defer q.fmut.Unlock()

	return len(q.files)
}

func (q *FileQueue) Get(nodeID string) (queuedBlock, bool) {
	q.fmut.Lock()
	defer q.fmut.Unlock()

	if !q.sorted {
		sort.Sort(q.files)
		q.sorted = true
	}

	for i := range q.files {
		qf := &q.files[i]

		q.amut.Lock()
		av := q.availability[qf.name]
		q.amut.Unlock()

		if len(av) == 0 {
			// Noone has the file we want; abort.
			if qf.remaining != len(qf.blocks) {
				// We have already started on this file; close it down
				close(qf.channel)
				if mon := qf.monitor; mon != nil {
					mon.FileDone()
				}
			}
			delete(q.queued, qf.name)
			q.deleteAt(i)
			return queuedBlock{}, false
		}

		for _, ni := range av {
			// Find and return the next block in the queue
			if ni == nodeID {
				for j, b := range qf.blocks {
					if !qf.activeBlocks[j] {
						qf.activeBlocks[j] = true
						qf.given++
						return queuedBlock{
							name:  qf.name,
							block: b,
							index: j,
						}, true
					}
				}
				break
			}
		}
	}

	// We found nothing to do
	return queuedBlock{}, false
}

func (q *FileQueue) Done(file string, offset int64, data []byte) {
	q.fmut.Lock()
	defer q.fmut.Unlock()

	c := content{
		offset: offset,
		data:   data,
	}
	for i := range q.files {
		qf := &q.files[i]

		if qf.name == file {
			if qf.monitor != nil && qf.remaining == len(qf.blocks) {
				err := qf.monitor.FileBegins(qf.channel)
				if err != nil {
					log.Printf("WARNING: %s: %v (not synced)", qf.name, err)
					delete(q.queued, qf.name)
					q.deleteAt(i)
					return
				}
			}

			qf.channel <- c
			qf.remaining--

			if qf.remaining == 0 {
				close(qf.channel)
				if qf.monitor != nil {
					err := qf.monitor.FileDone()
					if err != nil {
						log.Printf("WARNING: %s: %v", qf.name, err)
					}
				}
				delete(q.queued, qf.name)
				q.deleteAt(i)
			}
			return
		}
	}

	// We found nothing, might have errored out already
}

func (q *FileQueue) QueuedFiles() (files []string) {
	q.fmut.Lock()
	defer q.fmut.Unlock()

	for _, qf := range q.files {
		files = append(files, qf.name)
	}
	return
}

func (q *FileQueue) deleteAt(i int) {
	q.files = q.files[:i+copy(q.files[i:], q.files[i+1:])]
}

func (q *FileQueue) deleteFile(n string) {
	for i, file := range q.files {
		if n == file.name {
			q.deleteAt(i)
			delete(q.queued, file.name)
			return
		}
	}
}

func (q *FileQueue) SetAvailable(file string, nodes []string) {
	q.amut.Lock()
	defer q.amut.Unlock()

	q.availability[file] = nodes
}

func (q *FileQueue) RemoveAvailable(toRemove string) {
	q.amut.Lock()
	defer q.amut.Unlock()

	for file, nodes := range q.availability {
		for i, node := range nodes {
			if node == toRemove {
				q.availability[file] = nodes[:i+copy(nodes[i:], nodes[i+1:])]
				if len(q.availability[file]) == 0 {
					q.deleteFile(file)
				}
			}
			break
		}
	}
}
