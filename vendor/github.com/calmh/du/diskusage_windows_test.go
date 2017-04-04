package du

import "testing"

func TestDiskUsage(t *testing.T) {
	cases := []struct {
		path string
		ok   bool
	}{
		{"c:\\", true},
		{"c:\\windows", true},
		{"c:\\aux", false},
		{"c:\\does-not-exist-09sadkjhdsa98234bj23hgasd98", false},
	}
	for _, tc := range cases {
		res, err := Get(tc.path)
		if tc.ok {
			if err != nil {
				t.Errorf("Unexpected error Get(%q) => %v", tc.path, err)
			} else if res.TotalBytes == 0 || res.AvailBytes == 0 || res.FreeBytes == 0 {
				t.Errorf("Suspicious result Get(%q) => %v", tc.path, res)
			}
		} else if err == nil {
			t.Errorf("Unexpected nil error in Get(%q)", tc.path)
		}
	}
}
