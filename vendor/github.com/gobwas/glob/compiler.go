package glob

// TODO use constructor with all matchers, and to their structs private
// TODO glue multiple Text nodes (like after QuoteMeta)

import (
	"fmt"
	"github.com/gobwas/glob/match"
	"github.com/gobwas/glob/runes"
	"reflect"
)

func optimize(matcher match.Matcher) match.Matcher {
	switch m := matcher.(type) {

	case match.Any:
		if len(m.Separators) == 0 {
			return match.NewSuper()
		}

	case match.AnyOf:
		if len(m.Matchers) == 1 {
			return m.Matchers[0]
		}

		return m

	case match.List:
		if m.Not == false && len(m.List) == 1 {
			return match.NewText(string(m.List))
		}

		return m

	case match.BTree:
		m.Left = optimize(m.Left)
		m.Right = optimize(m.Right)

		r, ok := m.Value.(match.Text)
		if !ok {
			return m
		}

		leftNil := m.Left == nil
		rightNil := m.Right == nil

		if leftNil && rightNil {
			return match.NewText(r.Str)
		}

		_, leftSuper := m.Left.(match.Super)
		lp, leftPrefix := m.Left.(match.Prefix)

		_, rightSuper := m.Right.(match.Super)
		rs, rightSuffix := m.Right.(match.Suffix)

		if leftSuper && rightSuper {
			return match.NewContains(r.Str, false)
		}

		if leftSuper && rightNil {
			return match.NewSuffix(r.Str)
		}

		if rightSuper && leftNil {
			return match.NewPrefix(r.Str)
		}

		if leftNil && rightSuffix {
			return match.NewPrefixSuffix(r.Str, rs.Suffix)
		}

		if rightNil && leftPrefix {
			return match.NewPrefixSuffix(lp.Prefix, r.Str)
		}

		return m
	}

	return matcher
}

func glueMatchers(matchers []match.Matcher) match.Matcher {
	var (
		glued  []match.Matcher
		winner match.Matcher
	)
	maxLen := -1

	if m := glueAsEvery(matchers); m != nil {
		glued = append(glued, m)
		return m
	}

	if m := glueAsRow(matchers); m != nil {
		glued = append(glued, m)
		return m
	}

	for _, g := range glued {
		if l := g.Len(); l > maxLen {
			maxLen = l
			winner = g
		}
	}

	return winner
}

func glueAsRow(matchers []match.Matcher) match.Matcher {
	if len(matchers) <= 1 {
		return nil
	}

	var (
		c []match.Matcher
		l int
	)
	for _, matcher := range matchers {
		if ml := matcher.Len(); ml == -1 {
			return nil
		} else {
			c = append(c, matcher)
			l += ml
		}
	}

	return match.NewRow(l, c...)
}

func glueAsEvery(matchers []match.Matcher) match.Matcher {
	if len(matchers) <= 1 {
		return nil
	}

	var (
		hasAny    bool
		hasSuper  bool
		hasSingle bool
		min       int
		separator []rune
	)

	for i, matcher := range matchers {
		var sep []rune

		switch m := matcher.(type) {
		case match.Super:
			sep = []rune{}
			hasSuper = true

		case match.Any:
			sep = m.Separators
			hasAny = true

		case match.Single:
			sep = m.Separators
			hasSingle = true
			min++

		case match.List:
			if !m.Not {
				return nil
			}
			sep = m.List
			hasSingle = true
			min++

		default:
			return nil
		}

		// initialize
		if i == 0 {
			separator = sep
		}

		if runes.Equal(sep, separator) {
			continue
		}

		return nil
	}

	if hasSuper && !hasAny && !hasSingle {
		return match.NewSuper()
	}

	if hasAny && !hasSuper && !hasSingle {
		return match.NewAny(separator)
	}

	if (hasAny || hasSuper) && min > 0 && len(separator) == 0 {
		return match.NewMin(min)
	}

	every := match.NewEveryOf()

	if min > 0 {
		every.Add(match.NewMin(min))

		if !hasAny && !hasSuper {
			every.Add(match.NewMax(min))
		}
	}

	if len(separator) > 0 {
		every.Add(match.NewContains(string(separator), true))
	}

	return every
}

func minimizeMatchers(matchers []match.Matcher) []match.Matcher {
	var done match.Matcher
	var left, right, count int

	for l := 0; l < len(matchers); l++ {
		for r := len(matchers); r > l; r-- {
			if glued := glueMatchers(matchers[l:r]); glued != nil {
				var swap bool

				if done == nil {
					swap = true
				} else {
					cl, gl := done.Len(), glued.Len()
					swap = cl > -1 && gl > -1 && gl > cl
					swap = swap || count < r-l
				}

				if swap {
					done = glued
					left = l
					right = r
					count = r - l
				}
			}
		}
	}

	if done == nil {
		return matchers
	}

	next := append(append([]match.Matcher{}, matchers[:left]...), done)
	if right < len(matchers) {
		next = append(next, matchers[right:]...)
	}

	if len(next) == len(matchers) {
		return next
	}

	return minimizeMatchers(next)
}

func minimizeAnyOf(children []node) node {
	var nodes [][]node
	var min int
	var idx int
	for i, desc := range children {
		pat, ok := desc.(*nodePattern)
		if !ok {
			return nil
		}

		n := pat.children()
		ln := len(n)
		if len(nodes) == 0 || (ln < min) {
			min = ln
			idx = i
		}

		nodes = append(nodes, pat.children())
	}

	minNodes := nodes[idx]
	if idx+1 < len(nodes) {
		nodes = append(nodes[:idx], nodes[idx+1:]...)
	} else {
		nodes = nodes[:idx]
	}

	var commonLeft []node
	var commonLeftCount int
	for i, n := range minNodes {
		has := true
		for _, t := range nodes {
			if !reflect.DeepEqual(n, t[i]) {
				has = false
				break
			}
		}

		if has {
			commonLeft = append(commonLeft, n)
			commonLeftCount++
		} else {
			break
		}
	}

	var commonRight []node
	var commonRightCount int
	for i := min - 1; i > commonLeftCount-1; i-- {
		n := minNodes[i]
		has := true
		for _, t := range nodes {
			if !reflect.DeepEqual(n, t[len(t)-(min-i)]) {
				has = false
				break
			}
		}

		if has {
			commonRight = append(commonRight, n)
			commonRightCount++
		} else {
			break
		}
	}

	if commonLeftCount == 0 && commonRightCount == 0 {
		return nil
	}

	nodes = append(nodes, minNodes)
	nodes[len(nodes)-1], nodes[idx] = nodes[idx], nodes[len(nodes)-1]

	var result []node
	if commonLeftCount > 0 {
		result = append(result, &nodePattern{nodeImpl: nodeImpl{desc: commonLeft}})
	}

	var anyOf []node
	for _, n := range nodes {
		if commonLeftCount+commonRightCount == len(n) {
			anyOf = append(anyOf, nil)
		} else {
			anyOf = append(anyOf, &nodePattern{nodeImpl: nodeImpl{desc: n[commonLeftCount : len(n)-commonRightCount]}})
		}
	}

	anyOf = uniqueNodes(anyOf)
	if len(anyOf) == 1 {
		if anyOf[0] != nil {
			result = append(result, &nodePattern{nodeImpl: nodeImpl{desc: anyOf}})
		}
	} else {
		result = append(result, &nodeAnyOf{nodeImpl: nodeImpl{desc: anyOf}})
	}

	if commonRightCount > 0 {
		result = append(result, &nodePattern{nodeImpl: nodeImpl{desc: commonRight}})
	}

	return &nodePattern{nodeImpl: nodeImpl{desc: result}}
}

func uniqueNodes(nodes []node) (result []node) {
head:
	for _, n := range nodes {
		for _, e := range result {
			if reflect.DeepEqual(e, n) {
				continue head
			}
		}

		result = append(result, n)
	}

	return
}

func compileMatchers(matchers []match.Matcher) (match.Matcher, error) {
	if len(matchers) == 0 {
		return nil, fmt.Errorf("compile error: need at least one matcher")
	}

	if len(matchers) == 1 {
		return matchers[0], nil
	}

	if m := glueMatchers(matchers); m != nil {
		return m, nil
	}

	var (
		val match.Matcher
		idx int
	)
	maxLen := -1
	for i, matcher := range matchers {
		l := matcher.Len()
		if l >= maxLen {
			maxLen = l
			idx = i
			val = matcher
		}
	}

	left := matchers[:idx]
	var right []match.Matcher
	if len(matchers) > idx+1 {
		right = matchers[idx+1:]
	}

	var l, r match.Matcher
	var err error
	if len(left) > 0 {
		l, err = compileMatchers(left)
		if err != nil {
			return nil, err
		}
	}

	if len(right) > 0 {
		r, err = compileMatchers(right)
		if err != nil {
			return nil, err
		}
	}

	return match.NewBTree(val, l, r), nil
}

//func complexity(m match.Matcher) int {
//	var matchers []match.Matcher
//	var k int
//
//	switch matcher := m.(type) {
//
//	case match.Nothing:
//		return 0
//
//	case match.Max, match.Range, match.Suffix, match.Text:
//		return 1
//
//	case match.PrefixSuffix, match.Single, match.Row:
//		return 2
//
//	case match.Any, match.Contains, match.List, match.Min, match.Prefix, match.Super:
//		return 4
//
//	case match.BTree:
//		matchers = append(matchers, matcher.Value)
//		if matcher.Left != nil {
//			matchers = append(matchers, matcher.Left)
//		}
//		if matcher.Right != nil {
//			matchers = append(matchers, matcher.Right)
//		}
//		k = 1
//
//	case match.AnyOf:
//		matchers = matcher.Matchers
//		k = 1
//	case match.EveryOf:
//		matchers = matcher.Matchers
//		k = 1
//
//	default:
//		return 0
//	}
//
//	var sum int
//	for _, m := range matchers {
//		sum += complexity(m)
//	}
//
//	return sum * k
//}

func doAnyOf(n *nodeAnyOf, s []rune) (match.Matcher, error) {
	var matchers []match.Matcher
	for _, desc := range n.children() {
		if desc == nil {
			matchers = append(matchers, match.NewNothing())
			continue
		}

		m, err := do(desc, s)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, optimize(m))
	}

	return match.NewAnyOf(matchers...), nil
}

func do(leaf node, s []rune) (m match.Matcher, err error) {
	switch n := leaf.(type) {

	case *nodeAnyOf:
		// todo this could be faster on pattern_alternatives_combine_lite
		if n := minimizeAnyOf(n.children()); n != nil {
			return do(n, s)
		}

		var matchers []match.Matcher
		for _, desc := range n.children() {
			if desc == nil {
				matchers = append(matchers, match.NewNothing())
				continue
			}

			m, err := do(desc, s)
			if err != nil {
				return nil, err
			}
			matchers = append(matchers, optimize(m))
		}

		return match.NewAnyOf(matchers...), nil

	case *nodePattern:
		nodes := leaf.children()
		if len(nodes) == 0 {
			return match.NewNothing(), nil
		}

		var matchers []match.Matcher
		for _, desc := range nodes {
			m, err := do(desc, s)
			if err != nil {
				return nil, err
			}
			matchers = append(matchers, optimize(m))
		}

		m, err = compileMatchers(minimizeMatchers(matchers))
		if err != nil {
			return nil, err
		}

	case *nodeList:
		m = match.NewList([]rune(n.chars), n.not)

	case *nodeRange:
		m = match.NewRange(n.lo, n.hi, n.not)

	case *nodeAny:
		m = match.NewAny(s)

	case *nodeSuper:
		m = match.NewSuper()

	case *nodeSingle:
		m = match.NewSingle(s)

	case *nodeText:
		m = match.NewText(n.text)

	default:
		return nil, fmt.Errorf("could not compile tree: unknown node type")
	}

	return optimize(m), nil
}

func do2(node node, s []rune) ([]match.Matcher, error) {
	var result []match.Matcher

	switch n := node.(type) {

	case *nodePattern:
		ways := [][]match.Matcher{[]match.Matcher{}}

		for _, desc := range node.children() {
			variants, err := do2(desc, s)
			if err != nil {
				return nil, err
			}

			fmt.Println("variants pat", variants)

			for i, l := 0, len(ways); i < l; i++ {
				for i := 0; i < len(variants); i++ {
					o := optimize(variants[i])
					if i == len(variants)-1 {
						ways[i] = append(ways[i], o)
					} else {
						var w []match.Matcher
						copy(w, ways[i])
						ways = append(ways, append(w, o))
					}
				}
			}

			fmt.Println("ways pat", ways)
		}

		for _, matchers := range ways {
			c, err := compileMatchers(minimizeMatchers(matchers))
			if err != nil {
				return nil, err
			}
			result = append(result, c)
		}

	case *nodeAnyOf:
		ways := make([][]match.Matcher, len(node.children()))
		for _, desc := range node.children() {
			variants, err := do2(desc, s)
			if err != nil {
				return nil, err
			}

			fmt.Println("variants any", variants)

			for x, l := 0, len(ways); x < l; x++ {
				for i := 0; i < len(variants); i++ {
					o := optimize(variants[i])
					if i == len(variants)-1 {
						ways[x] = append(ways[x], o)
					} else {
						var w []match.Matcher
						copy(w, ways[x])
						ways = append(ways, append(w, o))
					}
				}
			}

			fmt.Println("ways any", ways)
		}

		for _, matchers := range ways {
			c, err := compileMatchers(minimizeMatchers(matchers))
			if err != nil {
				return nil, err
			}
			result = append(result, c)
		}

	case *nodeList:
		result = append(result, match.NewList([]rune(n.chars), n.not))

	case *nodeRange:
		result = append(result, match.NewRange(n.lo, n.hi, n.not))

	case *nodeAny:
		result = append(result, match.NewAny(s))

	case *nodeSuper:
		result = append(result, match.NewSuper())

	case *nodeSingle:
		result = append(result, match.NewSingle(s))

	case *nodeText:
		result = append(result, match.NewText(n.text))

	default:
		return nil, fmt.Errorf("could not compile tree: unknown node type")
	}

	for i, m := range result {
		result[i] = optimize(m)
	}

	return result, nil
}

func compile(ast *nodePattern, s []rune) (Glob, error) {
	//	ms, err := do2(ast, s)
	//	if err != nil {
	//		return nil, err
	//	}
	//	if len(ms) == 1 {
	//		return ms[0], nil
	//	} else {
	//		return match.NewAnyOf(ms), nil
	//	}

	g, err := do(ast, s)
	if err != nil {
		return nil, err
	}

	return g, nil
}
