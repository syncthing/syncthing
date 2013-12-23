package flags

func levenshtein(s string, t string) int {
	if len(s) == 0 {
		return len(t)
	}

	if len(t) == 0 {
		return len(s)
	}

	var l1, l2, l3 int

	if len(s) == 1 {
		l1 = len(t) + 1
	} else {
		l1 = levenshtein(s[1:len(s)-1], t) + 1
	}

	if len(t) == 1 {
		l2 = len(s) + 1
	} else {
		l2 = levenshtein(t[1:len(t)-1], s) + 1
	}

	l3 = levenshtein(s[1:len(s)], t[1:len(t)])

	if s[0] != t[0] {
		l3 += 1
	}

	if l2 < l1 {
		l1 = l2
	}

	if l1 < l3 {
		return l1
	}

	return l3
}

func closestChoice(cmd string, choices []string) (string, int) {
	if len(choices) == 0 {
		return "", 0
	}

	mincmd := -1
	mindist := -1

	for i, c := range choices {
		l := levenshtein(cmd, c)

		if mincmd < 0 || l < mindist {
			mindist = l
			mincmd = i
		}
	}

	return choices[mincmd], mindist
}
