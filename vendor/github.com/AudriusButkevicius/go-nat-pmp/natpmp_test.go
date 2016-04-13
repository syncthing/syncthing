package natpmp

import (
	"testing"
)

func TestNatPMP(t *testing.T) {
	client, err := NewClientForDefaultGateway()
	if err != nil {
		t.Errorf("NewClientForDefaultGateway() = %v,%v", client, err)
		return
	}
}
