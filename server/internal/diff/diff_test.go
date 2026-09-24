package diff

import "testing"

func TestLinesBasic(t *testing.T) {
	a := "line1\nline2\nline3"
	b := "line1\nlineX\nline3"
	segs := Lines(a, b)
	st := Summarize(segs)
	// line2 removed, lineX added, line1 & line3 equal (unchanged units count = 2).
	if st.Added != 1 || st.Removed != 1 {
		t.Errorf("expected +1 -1, got +%d -%d (%v)", st.Added, st.Removed, segs)
	}
}

func TestLinesIdentical(t *testing.T) {
	segs := Lines("same\ntext", "same\ntext")
	st := Summarize(segs)
	if st.Added != 0 || st.Removed != 0 {
		t.Errorf("identical text should have no changes, got %+v", st)
	}
}

func TestLinesInsertOnly(t *testing.T) {
	segs := Lines("a\nb", "a\nnew\nb")
	st := Summarize(segs)
	if st.Added != 1 || st.Removed != 0 {
		t.Errorf("expected one pure insertion, got %+v (%v)", st, segs)
	}
}

func TestTokens(t *testing.T) {
	segs := Tokens("the quick brown fox", "the slow brown fox")
	st := Summarize(segs)
	// "quick" -> "slow": one word removed, one added.
	if st.Added != 1 || st.Removed != 1 {
		t.Errorf("expected one word swap, got %+v (%v)", st, segs)
	}
}

func TestEmpty(t *testing.T) {
	if segs := Lines("", ""); len(segs) != 0 {
		t.Errorf("empty vs empty should be no segments, got %v", segs)
	}
	segs := Lines("", "hello")
	if st := Summarize(segs); st.Added != 1 {
		t.Errorf("empty->text should be one insert, got %+v", st)
	}
}
