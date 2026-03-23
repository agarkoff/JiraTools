package gitlab

import "strings"

// NormalizeDiff extracts meaningful +/- lines from a commit diff,
// excluding pom.xml files. Returns a set of lines for comparison.
func NormalizeDiff(diffs []DiffFile) []string {
	var lines []string
	for _, f := range diffs {
		// Skip pom.xml files
		if isPomXML(f.OldPath) || isPomXML(f.NewPath) {
			continue
		}
		for _, line := range strings.Split(f.Diff, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			// Only keep actual change lines (+ or -)
			if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
				// Skip diff headers
				if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") ||
					strings.HasPrefix(line, "@@") {
					continue
				}
				lines = append(lines, line)
			}
		}
	}
	return lines
}

// DiffSimilarity computes the similarity (0.0-1.0) between two normalized diffs.
// Uses Jaccard-like similarity on the line sets.
func DiffSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	setA := make(map[string]bool, len(a))
	for _, l := range a {
		setA[l] = true
	}
	setB := make(map[string]bool, len(b))
	for _, l := range b {
		setB[l] = true
	}

	intersection := 0
	for l := range setA {
		if setB[l] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

func isPomXML(path string) bool {
	return path == "pom.xml" || strings.HasSuffix(path, "/pom.xml")
}
