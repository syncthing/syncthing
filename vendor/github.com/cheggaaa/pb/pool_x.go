// +build linux darwin freebsd netbsd openbsd solaris dragonfly

package pb

import "fmt"

func (p *Pool) print(first bool) bool {
	p.m.Lock()
	defer p.m.Unlock()
	var out string
	if !first {
		out = fmt.Sprintf("\033[%dA", p.lastBarsCount)
	}
	isFinished := true
	for _, bar := range p.bars {
		if !bar.IsFinished() {
			isFinished = false
		}
		bar.Update()
		out += fmt.Sprintf("\r%s\n", bar.String())
	}
	if p.Output != nil {
		fmt.Fprint(p.Output, out)
	} else {
		fmt.Print(out)
	}
	p.lastBarsCount = len(p.bars)
	return isFinished
}
