package pb

import (
	"fmt"
	"time"
)

type Units int

const (
	// U_NO are default units, they represent a simple value and are not formatted at all.
	U_NO Units = iota
	// U_BYTES units are formatted in a human readable way (B, KiB, MiB, ...)
	U_BYTES
	// U_BYTES_DEC units are like U_BYTES, but base 10 (B, KB, MB, ...)
	U_BYTES_DEC
	// U_DURATION units are formatted in a human readable way (3h14m15s)
	U_DURATION
)

const (
	KiB = 1024
	MiB = 1048576
	GiB = 1073741824
	TiB = 1099511627776

	KB = 1e3
	MB = 1e6
	GB = 1e9
	TB = 1e12
)

func Format(i int64) *formatter {
	return &formatter{n: i}
}

type formatter struct {
	n      int64
	unit   Units
	width  int
	perSec bool
}

func (f *formatter) To(unit Units) *formatter {
	f.unit = unit
	return f
}

func (f *formatter) Width(width int) *formatter {
	f.width = width
	return f
}

func (f *formatter) PerSec() *formatter {
	f.perSec = true
	return f
}

func (f *formatter) String() (out string) {
	switch f.unit {
	case U_BYTES:
		out = formatBytes(f.n)
	case U_BYTES_DEC:
		out = formatBytesDec(f.n)
	case U_DURATION:
		out = formatDuration(f.n)
	default:
		out = fmt.Sprintf(fmt.Sprintf("%%%dd", f.width), f.n)
	}
	if f.perSec {
		out += "/s"
	}
	return
}

// Convert bytes to human readable string. Like 2 MiB, 64.2 KiB, 52 B
func formatBytes(i int64) (result string) {
	switch {
	case i >= TiB:
		result = fmt.Sprintf("%.02f TiB", float64(i)/TiB)
	case i >= GiB:
		result = fmt.Sprintf("%.02f GiB", float64(i)/GiB)
	case i >= MiB:
		result = fmt.Sprintf("%.02f MiB", float64(i)/MiB)
	case i >= KiB:
		result = fmt.Sprintf("%.02f KiB", float64(i)/KiB)
	default:
		result = fmt.Sprintf("%d B", i)
	}
	return
}

// Convert bytes to base-10 human readable string. Like 2 MB, 64.2 KB, 52 B
func formatBytesDec(i int64) (result string) {
	switch {
	case i >= TB:
		result = fmt.Sprintf("%.02f TB", float64(i)/TB)
	case i >= GB:
		result = fmt.Sprintf("%.02f GB", float64(i)/GB)
	case i >= MB:
		result = fmt.Sprintf("%.02f MB", float64(i)/MB)
	case i >= KB:
		result = fmt.Sprintf("%.02f KB", float64(i)/KB)
	default:
		result = fmt.Sprintf("%d B", i)
	}
	return
}

func formatDuration(n int64) (result string) {
	d := time.Duration(n)
	if d > time.Hour*24 {
		result = fmt.Sprintf("%dd", d/24/time.Hour)
		d -= (d / time.Hour / 24) * (time.Hour * 24)
	}
	result = fmt.Sprintf("%s%v", result, d)
	return
}
