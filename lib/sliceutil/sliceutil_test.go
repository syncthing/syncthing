package sliceutil_test

import (
	"testing"

	"github.com/syncthing/syncthing/lib/sliceutil"
	"golang.org/x/exp/slices"
)

func TestRemoveAndZero(t *testing.T) {
	a := []int{1, 2, 3, 4, 5}
	b := sliceutil.RemoveAndZero(a, 2)
	exp := []int{1, 2, 4, 5}
	if !slices.Equal(b, exp) {
		t.Errorf("got %v, expected %v", b, exp)
	}
	for _, e := range a {
		if e == 3 {
			t.Errorf("element should have been zeroed")
		}
	}
}
