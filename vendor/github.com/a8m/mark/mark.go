package mark

import "strings"

// Mark
type Mark struct {
	*parse
	Input string
}

// Mark options used to configure your Mark object
// set `Smartypants` and `Fractions` to true to enable
// smartypants and smartfractions rendering.
type Options struct {
	Gfm         bool
	Tables      bool
	Smartypants bool
	Fractions   bool
}

// DefaultOptions return an options struct with default configuration
// it's means that only Gfm, and Tables set to true.
func DefaultOptions() *Options {
	return &Options{
		Gfm:    true,
		Tables: true,
	}
}

// New return a new Mark
func New(input string, opts *Options) *Mark {
	// Preprocessing
	input = strings.Replace(input, "\t", "    ", -1)
	if opts == nil {
		opts = DefaultOptions()
	}
	return &Mark{
		Input: input,
		parse: newParse(input, opts),
	}
}

// parse and render input
func (m *Mark) Render() string {
	m.parse.parse()
	m.render()
	return m.output
}

// AddRenderFn let you pass NodeType, and RenderFn function
// and override the default Node rendering
func (m *Mark) AddRenderFn(typ NodeType, fn RenderFn) {
	m.renderFn[typ] = fn
}

// Staic render function
func Render(input string) string {
	m := New(input, nil)
	return m.Render()
}
