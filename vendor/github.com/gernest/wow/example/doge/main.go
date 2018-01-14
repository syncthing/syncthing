package main

import (
	"os"
	"time"

	"github.com/gernest/wow"
	"github.com/gernest/wow/spin"
)

func main() {
	w := wow.New(os.Stdout, spin.Get(spin.Dots), "Such Spins")
	w.Start()
	time.Sleep(2 * time.Second)
	w.Text("Very emojis").Spinner(spin.Get(spin.Hearts))
	time.Sleep(2 * time.Second)
	w.PersistWith(spin.Spinner{Frames: []string{"👍"}}, " Wow!")
}
