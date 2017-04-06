package config

import "testing"

func TestUnmarshalSize(t *testing.T) {
	cases := []struct {
		in  string
		ok  bool
		val float64
		pct bool
	}{
		// We accept upper case SI units
		{"5K", true, 5e3, false}, // even when they should be lower case
		{"4 M", true, 4e6, false},
		{"3G", true, 3e9, false},
		{"2 T", true, 2e12, false},
		// We accept lower case SI units out of user friendliness
		{"1 k", true, 1e3, false},
		{"2m", true, 2e6, false},
		{"3 g", true, 3e9, false},
		{"4t", true, 4e12, false},
		// We accept binary suffixes, with correct casing only
		{"  5 Ki  ", true, 5 * 1024, false},
		{" 4  Mi  ", true, 4 * 1024 * 1024, false},
		{" 3   Gi ", true, 3 * 1024 * 1024 * 1024, false},
		{"2    Ti ", true, 2 * 1024 * 1024 * 1024 * 1024, false},
		// Fractions are OK
		{"123.456 k", true, 123.456e3, false},
		{"0.1234 m", true, 0.1234e6, false},
		{"3.45 g", true, 3.45e9, false},
		// We don't parse negative numbers
		{"-1", false, 0, false},
		{"-1k", false, 0, false},
		{"-0.45g", false, 0, false},
		// We dont' accept other extra or random stuff
		{"100 KiBytes", false, 0, false},
		{"100 Kbps", false, 0, false},
		{"100 AU", false, 0, false},
		// Percentages are OK though
		{"1%", true, 1, true},
		{"200%", true, 200, true},   // even large ones
		{"200K%", false, 0, false},  // but not with suffixes
		{"2.34%", true, 2.34, true}, // fractions are A-ok
		// The empty string is a valid zero
		{"", true, 0, false},
		{"  ", true, 0, false},
	}

	for _, tc := range cases {
		var n Size
		err := n.UnmarshalText([]byte(tc.in))

		if !tc.ok {
			if err == nil {
				t.Errorf("Unexpected nil error in UnmarshalText(%q)", tc.in)
			}
			continue
		}

		if err != nil {
			t.Errorf("Unexpected error in UnmarshalText(%q): %v", tc.in, err)
			continue
		}
		if n.Value() > tc.val*1.001 || n.Value() < tc.val*0.999 {
			// Allow 0.1% slop due to floating point multiplication
			t.Errorf("Incorrect value in UnmarshalText(%q): %v, wanted %v", tc.in, n.value, tc.val)
		}
		if n.Percentage() != tc.pct {
			t.Errorf("Incorrect percentage bool in UnmarshalText(%q): %v, wanted %v", tc.in, n.percentage, tc.pct)
		}
	}
}

func TestMarshalSize(t *testing.T) {
	cases := []struct {
		in  string
		out string
	}{
		// SI units are normalized in case, spacing is normalized
		{"5K", "5 k"},
		{"4 M", "4 M"},
		{"3G", "3 G"},
		{"2 T", "2 T"},
		{"1 k", "1 k"},
		{"2m", "2 M"},
		{"3 g", "3 G"},
		{"4t", "4 T"},
		// We accept binary suffixes, with correct casing only
		{"  5 Ki  ", "5 Ki"},
		{" 4  Mi  ", "4 Mi"},
		{" 3   Gi ", "3 Gi"},
		{"2    Ti ", "2 Ti"},
		// Fractions are retained as is
		{"123.456 k", "123.456 k"},
		{"0.1234 m", "0.1234 M"},
		{"3.45 g", "3.45 G"},
		// Percentages are retained
		{"1%", "1 %"},
		{"200%", "200 %"},
		{"2.34%", "2.34 %"},
		// Empty
		{"  ", ""},
	}

	for _, tc := range cases {
		var n Size
		n.UnmarshalText([]byte(tc.in))
		out := n.String()

		if out != tc.out {
			t.Errorf("Incorrect Size(%q).String(): %q, wanted %q", tc.in, out, tc.out)
		}
	}
}
