package tui

import "github.com/sahilm/fuzzy"

// fuzzyMatchIndexes returns the indexes into labels whose text fuzzy-matches
// query, ordered best match first via github.com/sahilm/fuzzy's scoring. An
// empty query matches everything in original order — fuzzy.Find itself
// returns zero matches for an empty pattern, which would otherwise make an
// empty search box appear to filter out every item the moment it opens.
func fuzzyMatchIndexes(query string, labels []string) []int {
	if query == "" {
		idxs := make([]int, len(labels))
		for i := range labels {
			idxs[i] = i
		}
		return idxs
	}

	matches := fuzzy.Find(query, labels)
	idxs := make([]int, len(matches))
	for i, match := range matches {
		idxs[i] = match.Index
	}
	return idxs
}
