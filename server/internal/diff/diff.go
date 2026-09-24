// Package diff computes line- and token-level diffs between two strings using the
// classic Longest Common Subsequence (LCS) algorithm — the same core idea behind
// `git diff`. It has no dependencies; the algorithm is the lesson.
//
// See docs/learning/05-versioning-and-diff.md for the walkthrough.
package diff

import (
	"regexp"
	"strings"
)

// Op is a single diff operation kind.
type Op string

const (
	OpEqual  Op = "equal"  // token/line present unchanged in both
	OpInsert Op = "insert" // present only in the "to" side (added)
	OpDelete Op = "delete" // present only in the "from" side (removed)
)

// Segment is one contiguous run of a diff.
type Segment struct {
	Op   Op     `json:"op"`
	Text string `json:"text"`
}

// Lines returns a line-level diff from `a` to `b`.
func Lines(a, b string) []Segment {
	return diffTokens(splitLines(a), splitLines(b))
}

// Tokens returns a word/punctuation-level diff from `a` to `b`, for the
// fine-grained "which words changed" view.
func Tokens(a, b string) []Segment {
	return diffTokens(splitTokens(a), splitTokens(b))
}

// Stats summarizes a diff (added/removed/unchanged unit counts) for a quick
// headline like "+4 -1" in the UI.
type Stats struct {
	Added     int `json:"added"`
	Removed   int `json:"removed"`
	Unchanged int `json:"unchanged"`
}

// Summarize counts segments by op.
func Summarize(segs []Segment) Stats {
	var s Stats
	for _, seg := range segs {
		switch seg.Op {
		case OpInsert:
			s.Added++
		case OpDelete:
			s.Removed++
		case OpEqual:
			s.Unchanged++
		}
	}
	return s
}

// diffTokens is the shared engine: given two slices of units (lines or tokens),
// it computes the LCS and walks it into a segment list. Adjacent segments of the
// same op are merged so the output reads naturally.
func diffTokens(a, b []string) []Segment {
	lcs := lcsTable(a, b)
	// Walk the DP table from the bottom-right corner back to the origin,
	// reconstructing the edit sequence in reverse.
	var rev []Segment
	i, j := len(a), len(b)
	for i > 0 && j > 0 {
		switch {
		case a[i-1] == b[j-1]:
			rev = append(rev, Segment{OpEqual, a[i-1]})
			i--
			j--
		case lcs[i-1][j] >= lcs[i][j-1]:
			rev = append(rev, Segment{OpDelete, a[i-1]})
			i--
		default:
			rev = append(rev, Segment{OpInsert, b[j-1]})
			j--
		}
	}
	for i > 0 {
		rev = append(rev, Segment{OpDelete, a[i-1]})
		i--
	}
	for j > 0 {
		rev = append(rev, Segment{OpInsert, b[j-1]})
		j--
	}
	// rev is in reverse order; reverse it and merge adjacent same-op runs.
	return mergeAdjacent(reverse(rev))
}

// lcsTable builds the (m+1)x(n+1) dynamic-programming table where
// lcs[i][j] = length of the LCS of a[:i] and b[:j]. O(m*n) time and space.
func lcsTable(a, b []string) [][]int {
	m, n := len(a), len(b)
	t := make([][]int, m+1)
	for i := range t {
		t[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				t[i][j] = t[i-1][j-1] + 1
			} else if t[i-1][j] >= t[i][j-1] {
				t[i][j] = t[i-1][j]
			} else {
				t[i][j] = t[i][j-1]
			}
		}
	}
	return t
}

func reverse(s []Segment) []Segment {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// mergeAdjacent joins consecutive segments with the same op into one, inserting
// a newline between merged line-segments so the reconstructed text is readable.
func mergeAdjacent(segs []Segment) []Segment {
	if len(segs) == 0 {
		return segs
	}
	out := []Segment{segs[0]}
	for _, seg := range segs[1:] {
		last := &out[len(out)-1]
		if last.Op == seg.Op {
			last.Text += "\n" + seg.Text
		} else {
			out = append(out, seg)
		}
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// tokenSplitter keeps words and runs of punctuation/space as separate tokens so a
// token diff highlights word-level changes without splitting inside words.
var tokenSplitter = regexp.MustCompile(`\w+|[^\w\s]+|\s+`)

func splitTokens(s string) []string {
	if s == "" {
		return nil
	}
	return tokenSplitter.FindAllString(s, -1)
}
