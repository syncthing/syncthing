package kcp

import (
	"container/heap"
	"sync"
	"time"
)

var updater updateHeap

func init() {
	updater.init()
	go updater.updateTask()
}

// entry contains a session update info
type entry struct {
	sid uint32
	ts  time.Time
	s   *UDPSession
}

// a global heap managed kcp.flush() caller
type updateHeap struct {
	entries  []entry
	indices  map[uint32]int
	mu       sync.Mutex
	chWakeUp chan struct{}
}

func (h *updateHeap) Len() int           { return len(h.entries) }
func (h *updateHeap) Less(i, j int) bool { return h.entries[i].ts.Before(h.entries[j].ts) }
func (h *updateHeap) Swap(i, j int) {
	h.entries[i], h.entries[j] = h.entries[j], h.entries[i]
	h.indices[h.entries[i].sid] = i
	h.indices[h.entries[j].sid] = j
}

func (h *updateHeap) Push(x interface{}) {
	h.entries = append(h.entries, x.(entry))
	n := len(h.entries)
	h.indices[h.entries[n-1].sid] = n - 1
}

func (h *updateHeap) Pop() interface{} {
	n := len(h.entries)
	x := h.entries[n-1]
	h.entries[n-1] = entry{} // manual set nil for GC
	h.entries = h.entries[0 : n-1]
	delete(h.indices, x.sid)
	return x
}

func (h *updateHeap) init() {
	h.indices = make(map[uint32]int)
	h.chWakeUp = make(chan struct{}, 1)
}

func (h *updateHeap) addSession(s *UDPSession) {
	h.mu.Lock()
	heap.Push(h, entry{s.sid, time.Now(), s})
	h.mu.Unlock()
	h.wakeup()
}

func (h *updateHeap) removeSession(s *UDPSession) {
	h.mu.Lock()
	if idx, ok := h.indices[s.sid]; ok {
		heap.Remove(h, idx)
	}
	h.mu.Unlock()
}

func (h *updateHeap) wakeup() {
	select {
	case h.chWakeUp <- struct{}{}:
	default:
	}
}

func (h *updateHeap) updateTask() {
	var timer <-chan time.Time
	for {
		select {
		case <-timer:
		case <-h.chWakeUp:
		}

		h.mu.Lock()
		hlen := h.Len()
		now := time.Now()
		for i := 0; i < hlen; i++ {
			entry := heap.Pop(h).(entry)
			if now.After(entry.ts) {
				entry.ts = now.Add(entry.s.update())
				heap.Push(h, entry)
			} else {
				heap.Push(h, entry)
				break
			}
		}
		if h.Len() > 0 {
			timer = time.After(h.entries[0].ts.Sub(now))
		}
		h.mu.Unlock()
	}
}
