package files

import (
	"fmt"
	"time"

	"github.com/calmh/syncthing/cid"
)

type logEntry struct {
	time   time.Time
	method string
	cid    uint
	node   string
	nfiles int
}

func (l logEntry) String() string {
	return fmt.Sprintf("%v: %s cid:%d node:%s nfiles:%d", l.time, l.method, l.cid, l.node, l.nfiles)
}

var (
	debugLog  [10]logEntry
	debugNext int
	cm        *cid.Map
)

func SetCM(m *cid.Map) {
	cm = m
}

func log(method string, id uint, nfiles int) {
	e := logEntry{
		time:   time.Now(),
		method: method,
		cid:    id,
		nfiles: nfiles,
	}
	if cm != nil {
		e.node = cm.Name(id)
	}
	debugLog[debugNext] = e
	debugNext = (debugNext + 1) % len(debugLog)
}

func printLog() {
	l.Debugln("--- Consistency error ---")
	for _, e := range debugLog {
		l.Debugln(e)
	}
}
