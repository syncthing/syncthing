package main

import (
	"math/rand"

	"github.com/bkaradzic/go-lz4"

	// lz4's API matches snappy's, so we can easily see how it performs
	// lz4 "code.google.com/p/snappy-go/snappy"
)

var input = `
ADVENTURE I. A SCANDAL IN BOHEMIA

I.

To Sherlock Holmes she is always THE woman. I have seldom heard
him mention her under any other name. In his eyes she eclipses
and predominates the whole of her sex. It was not that he felt
any emotion akin to love for Irene Adler. All emotions, and that
one particularly, were abhorrent to his cold, precise but
admirably balanced mind. He was, I take it, the most perfect
reasoning and observing machine that the world has seen, but as a
lover he would have placed himself in a false position. He never
spoke of the softer passions, save with a gibe and a sneer. They
were admirable things for the observer--excellent for drawing the
veil from men's motives and actions. But for the trained reasoner
to admit such intrusions into his own delicate and finely
adjusted temperament was to introduce a distracting factor which
might throw a doubt upon all his mental results. Grit in a
sensitive instrument, or a crack in one of his own high-power
lenses, would not be more disturbing than a strong emotion in a
nature such as his. And yet there was but one woman to him, and
that woman was the late Irene Adler, of dubious and questionable
memory.

I had seen little of Holmes lately. My marriage had drifted us
away from each other. My own complete happiness, and the
home-centred interests which rise up around the man who first
finds himself master of his own establishment, were sufficient to
absorb all my attention, while Holmes, who loathed every form of
society with his whole Bohemian soul, remained in our lodgings in
Baker Street, buried among his old books, and alternating from
week to week between cocaine and ambition, the drowsiness of the
drug, and the fierce energy of his own keen nature. He was still,
as ever, deeply attracted by the study of crime, and occupied his
immense faculties and extraordinary powers of observation in
following out those clues, and clearing up those mysteries which
had been abandoned as hopeless by the official police. From time
to time I heard some vague account of his doings: of his summons
to Odessa in the case of the Trepoff murder, of his clearing up
of the singular tragedy of the Atkinson brothers at Trincomalee,
and finally of the mission which he had accomplished so
delicately and successfully for the reigning family of Holland.
Beyond these signs of his activity, however, which I merely
shared with all the readers of the daily press, I knew little of
my former friend and companion.
`

func main() {

	compressed, _ := lz4.Encode(nil, []byte(input))

	modified := make([]byte, len(compressed))

	for {
		copy(modified, compressed)
		for i := 0; i < 100; i++ {
			modified[rand.Intn(len(compressed)-4)+4] = byte(rand.Intn(256))
		}
		lz4.Decode(nil, modified)
	}

}
