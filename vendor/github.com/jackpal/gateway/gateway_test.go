package gateway

import "testing"

func TestGateway(t *testing.T) {
	ip, err := DiscoverGateway()
	if err != nil {
		t.Errorf("DiscoverGateway() = %v,%v", ip, err)
	}
}
