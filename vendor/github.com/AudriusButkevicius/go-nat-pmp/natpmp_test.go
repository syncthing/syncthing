package natpmp

import "testing"

func TestNatPMP(t *testing.T) {
	client, err := NewClientForDefaultGateway(0)
	if err != nil {
		t.Errorf("NewClientForDefaultGateway() = %v,%v", client, err)
		return
	}
}
