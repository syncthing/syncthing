package utp

import (
	"time"

	"github.com/anacrolix/missinggo"
)

type deadline struct {
	t      time.Time
	passed missinggo.Event
	timer  *time.Timer
}

func (me *deadline) set(t time.Time) {
	me.t = t
	me.passed.Clear()
	if me.timer != nil {
		me.timer.Stop()
	}
	me.update()
}

func (me *deadline) update() {
	if me.t.IsZero() {
		return
	}
	if time.Now().Before(me.t) {
		if me.timer == nil {
			me.timer = time.AfterFunc(me.t.Sub(time.Now()), me.callback)
		} else {
			me.timer.Reset(me.t.Sub(time.Now()))
		}
		return
	}
	me.passed.Set()
}

func (me *deadline) callback() {
	mu.Lock()
	defer mu.Unlock()
	me.update()
}

// This is embedded in Conn and Socket to provide deadline methods for
// net.Conn.
type connDeadlines struct {
	read, write deadline
}

func (c *connDeadlines) SetDeadline(t time.Time) error {
	mu.Lock()
	defer mu.Unlock()
	c.read.set(t)
	c.write.set(t)
	return nil
}

func (c *connDeadlines) SetReadDeadline(t time.Time) error {
	mu.Lock()
	defer mu.Unlock()
	c.read.set(t)
	return nil
}

func (c *connDeadlines) SetWriteDeadline(t time.Time) error {
	mu.Lock()
	defer mu.Unlock()
	c.write.set(t)
	return nil
}
