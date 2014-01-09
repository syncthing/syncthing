package model

import (
	"log"
	"sort"
	"sync"
	"time"
)

type Resolver interface {
	WhoHas(string) []string
}

type Monitor interface {
	FileBegins(<-chan content) error
	FileDone() error
}

type FileQueue struct {
	resolver Resolver
	files    queuedFileList
	lock     sync.Mutex
	sorted   bool
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
		if time.Since(q.files[i].nodesChecked) > 5*time.Second {
			// Refresh node list every now and then
			q.files[i].nodes = q.resolver.WhoHas(q.files[i].name)
		}

		if len(q.files[i].nodes) == 0 {
			// Noone has the file we want; abort.
			if q.files[i].remaining != len(q.files[i].blocks) {
				// We have already started on this file; close it down
				close(q.files[i].channel)
				if mon := q.files[i].monitor; mon != nil {
					mon.FileDone()
				}
			}
			q.deleteIndex(i)
			return queuedBlock{}, false
		}

		qf := q.files[i]
		for _, ni := range qf.nodes {
			// Find and return the next block in the queue
			if ni == nodeID {
				for j, b := range qf.blocks {
					if !qf.activeBlocks[j] {
						q.files[i].activeBlocks[j] = true
						q.files[i].given++
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
	for i, qf := range q.files {
		if qf.name == file {
			if qf.monitor != nil && qf.remaining == len(qf.blocks) {
				err := qf.monitor.FileBegins(qf.channel)
				if err != nil {
					log.Printf("WARNING: %s: %v (not synced)", qf.name, err)
					q.deleteIndex(i)
					return
				}
			}

			qf.channel <- c
			q.files[i].remaining--

			if q.files[i].remaining == 0 {
				close(qf.channel)
				q.deleteIndex(i)
				if qf.monitor != nil {
					err := qf.monitor.FileDone()
					if err != nil {
						log.Printf("WARNING: %s: %v", qf.name, err)
					}
				}
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

func (q *FileQueue) deleteIndex(i int) {
	q.files = q.files[:i+copy(q.files[i:], q.files[i+1:])]
}
