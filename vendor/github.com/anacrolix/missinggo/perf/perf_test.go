package perf

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/bradfitz/iter"
	"github.com/stretchr/testify/assert"
)

func TestTimer(t *testing.T) {
	tr := NewTimer()
	tr.Stop("hiyo")
	tr.Stop("hiyo")
	t.Log(em.Get("hiyo").(*buckets))
}

func BenchmarkStopWarm(b *testing.B) {
	tr := NewTimer()
	for range iter.N(b.N) {
		tr.Stop("a")
	}
}

func BenchmarkStopCold(b *testing.B) {
	tr := NewTimer()
	for i := range iter.N(b.N) {
		tr.Stop(strconv.FormatInt(int64(i), 10))
	}
}

func TestExponent(t *testing.T) {
	for _, c := range []struct {
		e int
		d time.Duration
	}{
		{-1, 10 * time.Millisecond},
		{-2, 5 * time.Millisecond},
		{-2, time.Millisecond},
		{-3, 500 * time.Microsecond},
		{-3, 100 * time.Microsecond},
	} {
		tr := NewTimer()
		time.Sleep(c.d)
		assert.Equal(t, c.e, bucketExponent(tr.Stop(fmt.Sprintf("%d", c.e))), "%s", c.d)
	}
	assert.Equal(t, `{">10ms": 1}`, em.Get("-1").String())
	assert.Equal(t, `{">1ms": 2}`, em.Get("-2").String())
	assert.Equal(t, `{">100µs": 2}`, em.Get("-3").String())
}
