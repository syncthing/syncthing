//DO NOT EDIT : this file was automatically generated.
package main

import (
	"os"
	"time"

	"github.com/gernest/wow"
	"github.com/gernest/wow/spin"
)

var all = []spin.Name{spin.Toggle6, spin.BouncingBall, spin.Balloon, spin.Toggle, spin.Toggle12, spin.Dots5, spin.Dots6, spin.Dots7, spin.GrowVertical, spin.Noise, spin.Toggle8, spin.SimpleDots, spin.BoxBounce2, spin.Arc, spin.BouncingBar, spin.Christmas, spin.Squish, spin.Triangle, spin.Arrow3, spin.Hearts, spin.Earth, spin.Dqpb, spin.Line2, spin.SquareCorners, spin.Toggle3, spin.Toggle5, spin.Monkey, spin.Clock, spin.Shark, spin.Weather, spin.Dots2, spin.Dots3, spin.Dots12, spin.CircleHalves, spin.Arrow, spin.Moon, spin.Flip, spin.Hamburger, spin.Bounce, spin.Circle, spin.Toggle9, spin.Toggle13, spin.Toggle10, spin.Runner, spin.Dots9, spin.Line, spin.Star, spin.CircleQuarters, spin.Arrow2, spin.Smiley, spin.Dots, spin.Dots4, spin.Pipe, spin.Balloon2, spin.Dots10, spin.Dots11, spin.SimpleDotsScrolling, spin.BoxBounce, spin.Toggle4, spin.Dots8, spin.Toggle2, spin.Toggle7, spin.Toggle11, spin.Pong, spin.Star2, spin.GrowHorizontal}

func main() {
	for _, v := range all {
		w := wow.New(os.Stdout, spin.Get(v), " "+v.String(), wow.ForceOutput)
		w.Start()
		time.Sleep(2)
		w.Persist()
	}
}
