package utp

import (
	"time"

	"github.com/anacrolix/missinggo"
)

type deadline struct {
	t      time.Time
	passed missinggo.Flag
	timer  *time.Timer
}

func (me *deadline) set(t time.Time) {
	me.passed.Set(false)
	me.t = t
	me.timer = time.AfterFunc(0, me.callback)
}

func (me *deadline) callback() {
	mu.Lock()
	defer mu.Unlock()
	if me.t.IsZero() {
		return
	}
	if time.Now().Before(me.t) {
		me.timer.Reset(me.t.Sub(time.Now()))
		return
	}
	me.passed.Set(true)
	cond.Broadcast()
}

// This is embedded in Conn to provide deadline methods for net.Conn. It
// tickles global mu and cond as required.
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
