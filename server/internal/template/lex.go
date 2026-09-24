package template

import (
	"fmt"
	"strings"
)

// chunkKind classifies a top-level piece of the template source.
type chunkKind int

const (
	chunkText   chunkKind = iota // literal text between tags
	chunkOutput                  // {{ ... }}  -> substitute a value
	chunkTag                     // {% ... %}  -> a statement (if/for/include/...)
)

// chunk is one lexed piece. `body` is the inner text with delimiters stripped and
// trimmed (for output/tag chunks) or the raw literal (for text chunks). `pos` is
// the byte offset in the source, used for error messages.
type chunk struct {
	kind chunkKind
	body string
	pos  int
}

// lex scans the template into an ordered list of chunks. This is a single linear
// pass: we look for the next "{{" or "{%", emit the text before it, then emit the
// delimited chunk. Everything after the last delimiter is trailing text.
//
// It deliberately does NOT understand if/for nesting — that's the parser's job.
// The lexer only knows three shapes of thing exist.
func lex(src string) ([]chunk, error) {
	var chunks []chunk
	i := 0
	for i < len(src) {
		// Find the next delimiter opening ("{{" or "{%"), whichever comes first.
		next := indexOfNextDelim(src[i:])
		if next < 0 {
			chunks = append(chunks, chunk{chunkText, src[i:], i})
			break
		}
		open := i + next
		if open > i {
			chunks = append(chunks, chunk{chunkText, src[i:open], i})
		}

		var closeDelim string
		var kind chunkKind
		if strings.HasPrefix(src[open:], "{{") {
			closeDelim, kind = "}}", chunkOutput
		} else {
			closeDelim, kind = "%}", chunkTag
		}

		end := strings.Index(src[open+2:], closeDelim)
		if end < 0 {
			return nil, fmt.Errorf("unclosed %q at offset %d (expected matching %q)",
				src[open:open+2], open, closeDelim)
		}
		inner := src[open+2 : open+2+end]
		chunks = append(chunks, chunk{kind, strings.TrimSpace(inner), open})
		i = open + 2 + end + 2
	}
	return chunks, nil
}

// indexOfNextDelim returns the byte offset of the earliest "{{" or "{%" in s, or
// -1 if neither appears.
func indexOfNextDelim(s string) int {
	a := strings.Index(s, "{{")
	b := strings.Index(s, "{%")
	switch {
	case a < 0:
		return b
	case b < 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}
