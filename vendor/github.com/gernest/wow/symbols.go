package wow

import "runtime"

// LogSymbol is a log severity level
type LogSymbol uint

// common log symbos
const (
	INFO LogSymbol = iota
	SUCCESS
	WARNING
	ERROR
)

func (s LogSymbol) String() string {
	if runtime.GOOS != "windows" {
		switch s {
		case INFO:
			return "ℹ"
		case SUCCESS:
			return "✔"
		case WARNING:
			return "⚠"
		case ERROR:
			return "✖"
		}
	} else {
		switch s {
		case INFO:
			return "i"
		case SUCCESS:
			return "v"
		case WARNING:
			return "!!"
		case ERROR:
			return "x"
		}
	}
	return ""
}
