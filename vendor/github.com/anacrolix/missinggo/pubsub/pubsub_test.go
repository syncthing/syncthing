package pubsub

import (
	"sync"
	"testing"

	"github.com/bradfitz/iter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoubleClose(t *testing.T) {
	ps := NewPubSub()
	ps.Close()
	ps.Close()
}

func testBroadcast(t testing.TB, subs, vals int) {
	ps := NewPubSub()
	var wg sync.WaitGroup
	for range iter.N(subs) {
		wg.Add(1)
		s := ps.Subscribe()
		go func() {
			defer wg.Done()
			var e int
			for i := range s.Values {
				assert.Equal(t, e, i.(int))
				e++
			}
			assert.Equal(t, vals, e)
		}()
	}
	for i := range iter.N(vals) {
		ps.Publish(i)
	}
	ps.Close()
	wg.Wait()
}

func TestBroadcast(t *testing.T) {
	testBroadcast(t, 100, 10)
}

func BenchmarkBroadcast(b *testing.B) {
	for range iter.N(b.N) {
		testBroadcast(b, 10, 1000)
	}
}

func TestCloseSubscription(t *testing.T) {
	ps := NewPubSub()
	ps.Publish(1)
	s := ps.Subscribe()
	select {
	case <-s.Values:
		t.FailNow()
	default:
	}
	ps.Publish(2)
	s2 := ps.Subscribe()
	ps.Publish(3)
	require.Equal(t, 2, <-s.Values)
	require.EqualValues(t, 3, <-s.Values)
	s.Close()
	_, ok := <-s.Values
	require.False(t, ok)
	ps.Publish(4)
	ps.Close()
	require.Equal(t, 3, <-s2.Values)
	require.Equal(t, 4, <-s2.Values)
	require.Nil(t, <-s2.Values)
	s2.Close()
}
