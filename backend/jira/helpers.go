package jira

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"jira-tools-web/models"
)

func FormatDisplayName(displayName string) string {
	parts := strings.Fields(displayName)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	name := []rune(parts[1])
	return fmt.Sprintf("%s %c.", parts[0], name[0])
}

func FormatAuthor(issue models.Issue) string {
	if issue.Fields.Creator == nil || issue.Fields.Creator.DisplayName == "" {
		return ""
	}
	return FormatDisplayName(issue.Fields.Creator.DisplayName)
}

func IsLinkedToStory(issue models.Issue) bool {
	if p := issue.Fields.Parent; p != nil {
		if p.Fields.IssueType.Name == "История" {
			return true
		}
	}

	for _, link := range issue.Fields.IssueLinks {
		if link.InwardIssue != nil && link.InwardIssue.Fields.IssueType.Name == "История" {
			return true
		}
		if link.OutwardIssue != nil && link.OutwardIssue.Fields.IssueType.Name == "История" {
			return true
		}
	}

	return false
}

func FormatHours(seconds int) string {
	if seconds == 0 {
		return "-"
	}
	hours := seconds / 3600
	return fmt.Sprintf("%dч", hours)
}

func IssueNum(key string) int {
	if idx := strings.LastIndex(key, "-"); idx >= 0 {
		n := 0
		for _, c := range key[idx+1:] {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		return n
	}
	return 0
}

// Statistical helpers

func Mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func MedianVal(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

func PercentileVal(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	k := (p / 100) * float64(len(sorted)-1)
	f := math.Floor(k)
	c := math.Ceil(k)
	if f == c {
		return sorted[int(k)]
	}
	return sorted[int(f)]*(c-k) + sorted[int(c)]*(k-f)
}

func StdDevVal(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	m := Mean(vals)
	sum := 0.0
	for _, v := range vals {
		d := v - m
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(vals)))
}

func Roundf(v float64, prec int) float64 {
	p := math.Pow(10, float64(prec))
	return math.Round(v*p) / p
}
