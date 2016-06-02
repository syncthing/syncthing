package missinggo

import (
	"testing"
	"time"

	"github.com/bradfitz/iter"
	"github.com/stretchr/testify/assert"
)

func TestTimerDrain(t *testing.T) {
	tr := time.NewTimer(0)
	<-tr.C
	select {
	case <-tr.C:
		assert.Fail(t, "shouldn't have received again on the the expired timer")
	default:
	}
	tr.Reset(1)
	select {
	case <-tr.C:
		assert.Fail(t, "received too soon")
	default:
	}
	time.Sleep(1)
	<-tr.C
	// Stop() should return false, as it just fired.
	assert.False(t, tr.Stop())
	tr.Reset(0)
	// Check we receive again after a Reset().
	<-tr.C
}

func TestTimerDoesNotFireAfterStop(t *testing.T) {
	t.Skip("the standard library implementation is broken")
	fail := make(chan struct{})
	done := make(chan struct{})
	defer close(done)
	for range iter.N(1000) {
		tr := time.NewTimer(0)
		tr.Stop()
		// There may or may not be a value in the channel now. But definitely
		// one should not be added after we receive it.
		select {
		case <-tr.C:
		default:
		}
		// Now set the timer to trigger in hour. It definitely shouldn't be
		// receivable now for an hour.
		tr.Reset(time.Hour)
		go func() {
			select {
			case <-tr.C:
				// As soon as the channel receives, notify failure.
				fail <- struct{}{}
			case <-done:
			}
		}()
	}
	select {
	case <-fail:
		t.FailNow()
	case <-time.After(100 * time.Millisecond):
	}
}
