package debug

import (
	"bytes"
	"fmt"
	"github.com/gobwas/glob/match"
	"math/rand"
)

func Graphviz(pattern string, m match.Matcher) string {
	return fmt.Sprintf(`digraph G {graph[label="%s"];%s}`, pattern, graphviz_internal(m, fmt.Sprintf("%x", rand.Int63())))
}

func graphviz_internal(m match.Matcher, id string) string {
	buf := &bytes.Buffer{}

	switch matcher := m.(type) {
	case match.BTree:
		fmt.Fprintf(buf, `"%s"[label="%s"];`, id, matcher.Value.String())
		for _, m := range []match.Matcher{matcher.Left, matcher.Right} {
			switch n := m.(type) {
			case nil:
				rnd := rand.Int63()
				fmt.Fprintf(buf, `"%x"[label="<nil>"];`, rnd)
				fmt.Fprintf(buf, `"%s"->"%x";`, id, rnd)

			default:
				sub := fmt.Sprintf("%x", rand.Int63())
				fmt.Fprintf(buf, `"%s"->"%s";`, id, sub)
				fmt.Fprintf(buf, graphviz_internal(n, sub))
			}
		}

	case match.AnyOf:
		fmt.Fprintf(buf, `"%s"[label="AnyOf"];`, id)
		for _, m := range matcher.Matchers {
			rnd := rand.Int63()
			fmt.Fprintf(buf, graphviz_internal(m, fmt.Sprintf("%x", rnd)))
			fmt.Fprintf(buf, `"%s"->"%x";`, id, rnd)
		}

	case match.EveryOf:
		fmt.Fprintf(buf, `"%s"[label="EveryOf"];`, id)
		for _, m := range matcher.Matchers {
			rnd := rand.Int63()
			fmt.Fprintf(buf, graphviz_internal(m, fmt.Sprintf("%x", rnd)))
			fmt.Fprintf(buf, `"%s"->"%x";`, id, rnd)
		}

	default:
		fmt.Fprintf(buf, `"%s"[label="%s"];`, id, m.String())
	}

	return buf.String()
}
