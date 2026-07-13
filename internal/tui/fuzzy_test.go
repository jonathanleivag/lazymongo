package tui

import "testing"

func TestFuzzyMatchIndexes_EmptyQueryReturnsAllInOriginalOrder(t *testing.T) {
	idxs := fuzzyMatchIndexes("", []string{"admin", "haddacloud-v2", "test"})
	if len(idxs) != 3 || idxs[0] != 0 || idxs[1] != 1 || idxs[2] != 2 {
		t.Fatalf("expected [0 1 2], got %v", idxs)
	}
}

func TestFuzzyMatchIndexes_NarrowsToSubsequenceMatches(t *testing.T) {
	idxs := fuzzyMatchIndexes("hdc", []string{"admin", "haddacloud-v2", "test"})
	if len(idxs) != 1 || idxs[0] != 1 {
		t.Fatalf("expected [1] (only 'haddacloud-v2' contains h,d,c in order), got %v", idxs)
	}
}

func TestFuzzyMatchIndexes_NoMatchesReturnsEmpty(t *testing.T) {
	idxs := fuzzyMatchIndexes("zzz", []string{"admin", "test"})
	if len(idxs) != 0 {
		t.Fatalf("expected no matches, got %v", idxs)
	}
}

func TestFuzzyMatchIndexes_BetterMatchRanksFirst(t *testing.T) {
	idxs := fuzzyMatchIndexes("test", []string{"testing", "test"})
	if len(idxs) != 2 || idxs[0] != 1 {
		t.Fatalf("expected the exact match 'test' (index 1) ranked first, got %v", idxs)
	}
}
