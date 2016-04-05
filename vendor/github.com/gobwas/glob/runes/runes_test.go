package runes

import (
	"strings"
	"testing"
)

type indexTest struct {
	s   []rune
	sep []rune
	out int
}

type equalTest struct {
	a   []rune
	b   []rune
	out bool
}

func newIndexTest(s, sep string, out int) indexTest {
	return indexTest{[]rune(s), []rune(sep), out}
}
func newEqualTest(s, sep string, out bool) equalTest {
	return equalTest{[]rune(s), []rune(sep), out}
}

var dots = "1....2....3....4"

var indexTests = []indexTest{
	newIndexTest("", "", 0),
	newIndexTest("", "a", -1),
	newIndexTest("", "foo", -1),
	newIndexTest("fo", "foo", -1),
	newIndexTest("foo", "foo", 0),
	newIndexTest("oofofoofooo", "f", 2),
	newIndexTest("oofofoofooo", "foo", 4),
	newIndexTest("barfoobarfoo", "foo", 3),
	newIndexTest("foo", "", 0),
	newIndexTest("foo", "o", 1),
	newIndexTest("abcABCabc", "A", 3),
	// cases with one byte strings - test special case in Index()
	newIndexTest("", "a", -1),
	newIndexTest("x", "a", -1),
	newIndexTest("x", "x", 0),
	newIndexTest("abc", "a", 0),
	newIndexTest("abc", "b", 1),
	newIndexTest("abc", "c", 2),
	newIndexTest("abc", "x", -1),
}

var lastIndexTests = []indexTest{
	newIndexTest("", "", 0),
	newIndexTest("", "a", -1),
	newIndexTest("", "foo", -1),
	newIndexTest("fo", "foo", -1),
	newIndexTest("foo", "foo", 0),
	newIndexTest("foo", "f", 0),
	newIndexTest("oofofoofooo", "f", 7),
	newIndexTest("oofofoofooo", "foo", 7),
	newIndexTest("barfoobarfoo", "foo", 9),
	newIndexTest("foo", "", 3),
	newIndexTest("foo", "o", 2),
	newIndexTest("abcABCabc", "A", 3),
	newIndexTest("abcABCabc", "a", 6),
}

var indexAnyTests = []indexTest{
	newIndexTest("", "", -1),
	newIndexTest("", "a", -1),
	newIndexTest("", "abc", -1),
	newIndexTest("a", "", -1),
	newIndexTest("a", "a", 0),
	newIndexTest("aaa", "a", 0),
	newIndexTest("abc", "xyz", -1),
	newIndexTest("abc", "xcz", 2),
	newIndexTest("a☺b☻c☹d", "uvw☻xyz", 3),
	newIndexTest("aRegExp*", ".(|)*+?^$[]", 7),
	newIndexTest(dots+dots+dots, " ", -1),
}

// Execute f on each test case.  funcName should be the name of f; it's used
// in failure reports.
func runIndexTests(t *testing.T, f func(s, sep []rune) int, funcName string, testCases []indexTest) {
	for _, test := range testCases {
		actual := f(test.s, test.sep)
		if actual != test.out {
			t.Errorf("%s(%q,%q) = %v; want %v", funcName, test.s, test.sep, actual, test.out)
		}
	}
}

func TestIndex(t *testing.T)     { runIndexTests(t, Index, "Index", indexTests) }
func TestLastIndex(t *testing.T) { runIndexTests(t, LastIndex, "LastIndex", lastIndexTests) }
func TestIndexAny(t *testing.T)  { runIndexTests(t, IndexAny, "IndexAny", indexAnyTests) }

var equalTests = []equalTest{
	newEqualTest("a", "a", true),
	newEqualTest("a", "b", false),
	newEqualTest("a☺b☻c☹d", "uvw☻xyz", false),
	newEqualTest("a☺b☻c☹d", "a☺b☻c☹d", true),
}

func TestEqual(t *testing.T) {
	for _, test := range equalTests {
		actual := Equal(test.a, test.b)
		if actual != test.out {
			t.Errorf("Equal(%q,%q) = %v; want %v", test.a, test.b, actual, test.out)
		}
	}
}

func BenchmarkLastIndexRunes(b *testing.B) {
	r := []rune("abcdef")
	n := []rune("cd")

	for i := 0; i < b.N; i++ {
		LastIndex(r, n)
	}
}
func BenchmarkLastIndexStrings(b *testing.B) {
	r := "abcdef"
	n := "cd"

	for i := 0; i < b.N; i++ {
		strings.LastIndex(r, n)
	}
}

func BenchmarkIndexAnyRunes(b *testing.B) {
	s := []rune("...b...")
	c := []rune("abc")

	for i := 0; i < b.N; i++ {
		IndexAny(s, c)
	}
}
func BenchmarkIndexAnyStrings(b *testing.B) {
	s := "...b..."
	c := "abc"

	for i := 0; i < b.N; i++ {
		strings.IndexAny(s, c)
	}
}

func BenchmarkIndexRuneRunes(b *testing.B) {
	s := []rune("...b...")
	r := 'b'

	for i := 0; i < b.N; i++ {
		IndexRune(s, r)
	}
}
func BenchmarkIndexRuneStrings(b *testing.B) {
	s := "...b..."
	r := 'b'

	for i := 0; i < b.N; i++ {
		strings.IndexRune(s, r)
	}
}

func BenchmarkIndexRunes(b *testing.B) {
	r := []rune("abcdef")
	n := []rune("cd")

	for i := 0; i < b.N; i++ {
		Index(r, n)
	}
}
func BenchmarkIndexStrings(b *testing.B) {
	r := "abcdef"
	n := "cd"

	for i := 0; i < b.N; i++ {
		strings.Index(r, n)
	}
}

func BenchmarkEqualRunes(b *testing.B) {
	x := []rune("abc")
	y := []rune("abc")

	for i := 0; i < b.N; i++ {
		if Equal(x, y) {
			continue
		}
	}
}

func BenchmarkEqualStrings(b *testing.B) {
	x := "abc"
	y := "abc"

	for i := 0; i < b.N; i++ {
		if x == y {
			continue
		}
	}
}

func BenchmarkNotEqualRunes(b *testing.B) {
	x := []rune("abc")
	y := []rune("abcd")

	for i := 0; i < b.N; i++ {
		if Equal(x, y) {
			continue
		}
	}
}

func BenchmarkNotEqualStrings(b *testing.B) {
	x := "abc"
	y := "abcd"

	for i := 0; i < b.N; i++ {
		if x == y {
			continue
		}
	}
}
