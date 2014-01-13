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
	lock         sync.Mutex
	sorted       bool
	availability map[string][]string
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

func (q *FileQueue) Add(name string, blocks []Block, monitor Monitor) {
	q.lock.Lock()
	defer q.lock.Unlock()

	q.files = append(q.files, queuedFile{
		name:         name,
		blocks:       blocks,
		activeBlocks: make([]bool, len(blocks)),
		remaining:    len(blocks),
		channel:      make(chan content),
		monitor:      monitor,
	})
	q.sorted = false
}

func (q *FileQueue) Len() int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return len(q.files)
}

func (q *FileQueue) Get(nodeID string) (queuedBlock, bool) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if !q.sorted {
		sort.Sort(q.files)
		q.sorted = true
	}

	for i := range q.files {
		qf := &q.files[i]

		if len(q.availability[qf.name]) == 0 {
			// Noone has the file we want; abort.
			if qf.remaining != len(qf.blocks) {
				// We have already started on this file; close it down
				close(qf.channel)
				if mon := qf.monitor; mon != nil {
					mon.FileDone()
				}
			}
			q.deleteAt(i)
			return queuedBlock{}, false
		}

		for _, ni := range q.availability[qf.name] {
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
	q.lock.Lock()
	defer q.lock.Unlock()

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
				q.deleteAt(i)
			}
			return
		}
	}
	panic("unreachable")
}

func (q *FileQueue) Queued(file string) bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	for _, qf := range q.files {
		if qf.name == file {
			return true
		}
	}
	return false
}

func (q *FileQueue) QueuedFiles() (files []string) {
	q.lock.Lock()
	defer q.lock.Unlock()

	for _, qf := range q.files {
		files = append(files, qf.name)
	}
	return
}

func (q *FileQueue) deleteAt(i int) {
	q.files = q.files[:i+copy(q.files[i:], q.files[i+1:])]
}

func (q *FileQueue) SetAvailable(file, node string) {
	q.lock.Lock()
	defer q.lock.Unlock()
	if q.availability == nil {
		q.availability = make(map[string][]string)
	}
	q.availability[file] = []string{node}
}

func (q *FileQueue) AddAvailable(file, node string) {
	q.lock.Lock()
	defer q.lock.Unlock()
	if q.availability == nil {
		q.availability = make(map[string][]string)
	}
	q.availability[file] = append(q.availability[file], node)
}
