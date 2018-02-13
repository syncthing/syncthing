// +build windows

package pb

import (
	"fmt"
	"log"
)

func (p *Pool) print(first bool) bool {
	p.m.Lock()
	defer p.m.Unlock()
	var out string
	if !first {
		coords, err := getCursorPos()
		if err != nil {
			log.Panic(err)
		}
		coords.Y -= int16(p.lastBarsCount)
		if coords.Y < 0 {
			coords.Y = 0
		}
		coords.X = 0

		err = setCursorPos(coords)
		if err != nil {
			log.Panic(err)
		}
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
