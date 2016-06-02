package missinggo

// A RunLengthEncoder counts successive duplicate elements and emits the
// element and the run length when the element changes or the encoder is
// flushed.
type RunLengthEncoder interface {
	// Add a series of identical elements to the stream.
	Append(element interface{}, count uint64)
	// Emit the current element and its count if non-zero without waiting for
	// the element to change.
	Flush()
}

type runLengthEncoder struct {
	eachRun func(element interface{}, count uint64)
	element interface{}
	count   uint64
}

// Creates a new RunLengthEncoder. eachRun is called when an element and its
// count is emitted, per the RunLengthEncoder interface.
func NewRunLengthEncoder(eachRun func(element interface{}, count uint64)) RunLengthEncoder {
	return &runLengthEncoder{
		eachRun: eachRun,
	}
}

func (me *runLengthEncoder) Append(element interface{}, count uint64) {
	if element == me.element {
		me.count += count
		return
	}
	if me.count != 0 {
		me.eachRun(me.element, me.count)
	}
	me.count = count
	me.element = element
}

func (me *runLengthEncoder) Flush() {
	if me.count == 0 {
		return
	}
	me.eachRun(me.element, me.count)
	me.count = 0
}
