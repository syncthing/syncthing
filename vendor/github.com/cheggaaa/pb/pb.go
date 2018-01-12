// Simple console progress bars
package pb

import (
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// Current version
const Version = "1.0.19"

const (
	// Default refresh rate - 200ms
	DEFAULT_REFRESH_RATE = time.Millisecond * 200
	FORMAT               = "[=>-]"
)

// DEPRECATED
// variables for backward compatibility, from now do not work
// use pb.Format and pb.SetRefreshRate
var (
	DefaultRefreshRate                         = DEFAULT_REFRESH_RATE
	BarStart, BarEnd, Empty, Current, CurrentN string
)

// Create new progress bar object
func New(total int) *ProgressBar {
	return New64(int64(total))
}

// Create new progress bar object using int64 as total
func New64(total int64) *ProgressBar {
	pb := &ProgressBar{
		Total:         total,
		RefreshRate:   DEFAULT_REFRESH_RATE,
		ShowPercent:   true,
		ShowCounters:  true,
		ShowBar:       true,
		ShowTimeLeft:  true,
		ShowFinalTime: true,
		Units:         U_NO,
		ManualUpdate:  false,
		finish:        make(chan struct{}),
	}
	return pb.Format(FORMAT)
}

// Create new object and start
func StartNew(total int) *ProgressBar {
	return New(total).Start()
}

// Callback for custom output
// For example:
// bar.Callback = func(s string) {
//     mySuperPrint(s)
// }
//
type Callback func(out string)

type ProgressBar struct {
	current  int64 // current must be first member of struct (https://code.google.com/p/go/issues/detail?id=5278)
	previous int64

	Total                            int64
	RefreshRate                      time.Duration
	ShowPercent, ShowCounters        bool
	ShowSpeed, ShowTimeLeft, ShowBar bool
	ShowFinalTime                    bool
	Output                           io.Writer
	Callback                         Callback
	NotPrint                         bool
	Units                            Units
	Width                            int
	ForceWidth                       bool
	ManualUpdate                     bool
	AutoStat                         bool

	// Default width for the time box.
	UnitsWidth   int
	TimeBoxWidth int

	finishOnce sync.Once //Guards isFinish
	finish     chan struct{}
	isFinish   bool

	startTime  time.Time
	startValue int64

	changeTime time.Time

	prefix, postfix string

	mu        sync.Mutex
	lastPrint string

	BarStart string
	BarEnd   string
	Empty    string
	Current  string
	CurrentN string

	AlwaysUpdate bool
}

// Start print
func (pb *ProgressBar) Start() *ProgressBar {
	pb.startTime = time.Now()
	pb.startValue = atomic.LoadInt64(&pb.current)
	if pb.Total == 0 {
		pb.ShowTimeLeft = false
		pb.ShowPercent = false
		pb.AutoStat = false
	}
	if !pb.ManualUpdate {
		pb.Update() // Initial printing of the bar before running the bar refresher.
		go pb.refresher()
	}
	return pb
}

// Increment current value
func (pb *ProgressBar) Increment() int {
	return pb.Add(1)
}

// Get current value
func (pb *ProgressBar) Get() int64 {
	c := atomic.LoadInt64(&pb.current)
	return c
}

// Set current value
func (pb *ProgressBar) Set(current int) *ProgressBar {
	return pb.Set64(int64(current))
}

// Set64 sets the current value as int64
func (pb *ProgressBar) Set64(current int64) *ProgressBar {
	atomic.StoreInt64(&pb.current, current)
	return pb
}

// Add to current value
func (pb *ProgressBar) Add(add int) int {
	return int(pb.Add64(int64(add)))
}

func (pb *ProgressBar) Add64(add int64) int64 {
	return atomic.AddInt64(&pb.current, add)
}

// Set prefix string
func (pb *ProgressBar) Prefix(prefix string) *ProgressBar {
	pb.prefix = prefix
	return pb
}

// Set postfix string
func (pb *ProgressBar) Postfix(postfix string) *ProgressBar {
	pb.postfix = postfix
	return pb
}

// Set custom format for bar
// Example: bar.Format("[=>_]")
// Example: bar.Format("[\x00=\x00>\x00-\x00]") // \x00 is the delimiter
func (pb *ProgressBar) Format(format string) *ProgressBar {
	var formatEntries []string
	if utf8.RuneCountInString(format) == 5 {
		formatEntries = strings.Split(format, "")
	} else {
		formatEntries = strings.Split(format, "\x00")
	}
	if len(formatEntries) == 5 {
		pb.BarStart = formatEntries[0]
		pb.BarEnd = formatEntries[4]
		pb.Empty = formatEntries[3]
		pb.Current = formatEntries[1]
		pb.CurrentN = formatEntries[2]
	}
	return pb
}

// Set bar refresh rate
func (pb *ProgressBar) SetRefreshRate(rate time.Duration) *ProgressBar {
	pb.RefreshRate = rate
	return pb
}

// Set units
// bar.SetUnits(U_NO) - by default
// bar.SetUnits(U_BYTES) - for Mb, Kb, etc
func (pb *ProgressBar) SetUnits(units Units) *ProgressBar {
	pb.Units = units
	return pb
}

// Set max width, if width is bigger than terminal width, will be ignored
func (pb *ProgressBar) SetMaxWidth(width int) *ProgressBar {
	pb.Width = width
	pb.ForceWidth = false
	return pb
}

// Set bar width
func (pb *ProgressBar) SetWidth(width int) *ProgressBar {
	pb.Width = width
	pb.ForceWidth = true
	return pb
}

// End print
func (pb *ProgressBar) Finish() {
	//Protect multiple calls
	pb.finishOnce.Do(func() {
		close(pb.finish)
		pb.write(atomic.LoadInt64(&pb.current))
		pb.mu.Lock()
		defer pb.mu.Unlock()
		switch {
		case pb.Output != nil:
			fmt.Fprintln(pb.Output)
		case !pb.NotPrint:
			fmt.Println()
		}
		pb.isFinish = true
	})
}

// IsFinished return boolean
func (pb *ProgressBar) IsFinished() bool {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return pb.isFinish
}

// End print and write string 'str'
func (pb *ProgressBar) FinishPrint(str string) {
	pb.Finish()
	if pb.Output != nil {
		fmt.Fprintln(pb.Output, str)
	} else {
		fmt.Println(str)
	}
}

// implement io.Writer
func (pb *ProgressBar) Write(p []byte) (n int, err error) {
	n = len(p)
	pb.Add(n)
	return
}

// implement io.Reader
func (pb *ProgressBar) Read(p []byte) (n int, err error) {
	n = len(p)
	pb.Add(n)
	return
}

// Create new proxy reader over bar
// Takes io.Reader or io.ReadCloser
func (pb *ProgressBar) NewProxyReader(r io.Reader) *Reader {
	return &Reader{r, pb}
}

func (pb *ProgressBar) write(current int64) {
	width := pb.GetWidth()

	var percentBox, countersBox, timeLeftBox, speedBox, barBox, end, out string

	// percents
	if pb.ShowPercent {
		var percent float64
		if pb.Total > 0 {
			percent = float64(current) / (float64(pb.Total) / float64(100))
		} else {
			percent = float64(current) / float64(100)
		}
		percentBox = fmt.Sprintf(" %6.02f%%", percent)
	}

	// counters
	if pb.ShowCounters {
		current := Format(current).To(pb.Units).Width(pb.UnitsWidth)
		if pb.Total > 0 {
			total := Format(pb.Total).To(pb.Units).Width(pb.UnitsWidth)
			countersBox = fmt.Sprintf(" %s / %s ", current, total)
		} else {
			countersBox = fmt.Sprintf(" %s / ? ", current)
		}
	}

	// time left
	pb.mu.Lock()
	currentFromStart := current - pb.startValue
	fromStart := time.Now().Sub(pb.startTime)
	lastChangeTime := pb.changeTime
	fromChange := lastChangeTime.Sub(pb.startTime)
	pb.mu.Unlock()
	select {
	case <-pb.finish:
		if pb.ShowFinalTime {
			var left time.Duration
			left = (fromStart / time.Second) * time.Second
			timeLeftBox = fmt.Sprintf(" %s", left.String())
		}
	default:
		if pb.ShowTimeLeft && currentFromStart > 0 {
			perEntry := fromChange / time.Duration(currentFromStart)
			var left time.Duration
			if pb.Total > 0 {
				left = time.Duration(pb.Total-currentFromStart) * perEntry
				left -= time.Since(lastChangeTime)
				left = (left / time.Second) * time.Second
			} else {
				left = time.Duration(currentFromStart) * perEntry
				left = (left / time.Second) * time.Second
			}
			if left > 0 {
				timeLeft := Format(int64(left)).To(U_DURATION).String()
				timeLeftBox = fmt.Sprintf(" %s", timeLeft)
			}
		}
	}

	if len(timeLeftBox) < pb.TimeBoxWidth {
		timeLeftBox = fmt.Sprintf("%s%s", strings.Repeat(" ", pb.TimeBoxWidth-len(timeLeftBox)), timeLeftBox)
	}

	// speed
	if pb.ShowSpeed && currentFromStart > 0 {
		fromStart := time.Now().Sub(pb.startTime)
		speed := float64(currentFromStart) / (float64(fromStart) / float64(time.Second))
		speedBox = " " + Format(int64(speed)).To(pb.Units).Width(pb.UnitsWidth).PerSec().String()
	}

	barWidth := escapeAwareRuneCountInString(countersBox + pb.BarStart + pb.BarEnd + percentBox + timeLeftBox + speedBox + pb.prefix + pb.postfix)
	// bar
	if pb.ShowBar {
		size := width - barWidth
		if size > 0 {
			if pb.Total > 0 {
				curSize := int(math.Ceil((float64(current) / float64(pb.Total)) * float64(size)))
				emptySize := size - curSize
				barBox = pb.BarStart
				if emptySize < 0 {
					emptySize = 0
				}
				if curSize > size {
					curSize = size
				}

				cursorLen := escapeAwareRuneCountInString(pb.Current)
				if emptySize <= 0 {
					barBox += strings.Repeat(pb.Current, curSize/cursorLen)
				} else if curSize > 0 {
					cursorEndLen := escapeAwareRuneCountInString(pb.CurrentN)
					cursorRepetitions := (curSize - cursorEndLen) / cursorLen
					barBox += strings.Repeat(pb.Current, cursorRepetitions)
					barBox += pb.CurrentN
				}

				emptyLen := escapeAwareRuneCountInString(pb.Empty)
				barBox += strings.Repeat(pb.Empty, emptySize/emptyLen)
				barBox += pb.BarEnd
			} else {
				pos := size - int(current)%int(size)
				barBox = pb.BarStart
				if pos-1 > 0 {
					barBox += strings.Repeat(pb.Empty, pos-1)
				}
				barBox += pb.Current
				if size-pos-1 > 0 {
					barBox += strings.Repeat(pb.Empty, size-pos-1)
				}
				barBox += pb.BarEnd
			}
		}
	}

	// check len
	out = pb.prefix + countersBox + barBox + percentBox + speedBox + timeLeftBox + pb.postfix
	if cl := escapeAwareRuneCountInString(out); cl < width {
		end = strings.Repeat(" ", width-cl)
	}

	// and print!
	pb.mu.Lock()
	pb.lastPrint = out + end
	isFinish := pb.isFinish
	pb.mu.Unlock()
	switch {
	case isFinish:
		return
	case pb.Output != nil:
		fmt.Fprint(pb.Output, "\r"+out+end)
	case pb.Callback != nil:
		pb.Callback(out + end)
	case !pb.NotPrint:
		fmt.Print("\r" + out + end)
	}
}

// GetTerminalWidth - returns terminal width for all platforms.
func GetTerminalWidth() (int, error) {
	return terminalWidth()
}

func (pb *ProgressBar) GetWidth() int {
	if pb.ForceWidth {
		return pb.Width
	}

	width := pb.Width
	termWidth, _ := terminalWidth()
	if width == 0 || termWidth <= width {
		width = termWidth
	}

	return width
}

// Write the current state of the progressbar
func (pb *ProgressBar) Update() {
	c := atomic.LoadInt64(&pb.current)
	p := atomic.LoadInt64(&pb.previous)
	if p != c {
		pb.mu.Lock()
		pb.changeTime = time.Now()
		pb.mu.Unlock()
		atomic.StoreInt64(&pb.previous, c)
	}
	pb.write(c)
	if pb.AutoStat {
		if c == 0 {
			pb.startTime = time.Now()
			pb.startValue = 0
		} else if c >= pb.Total && pb.isFinish != true {
			pb.Finish()
		}
	}
}

// String return the last bar print
func (pb *ProgressBar) String() string {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return pb.lastPrint
}

// Internal loop for refreshing the progressbar
func (pb *ProgressBar) refresher() {
	for {
		select {
		case <-pb.finish:
			return
		case <-time.After(pb.RefreshRate):
			pb.Update()
		}
	}
}
