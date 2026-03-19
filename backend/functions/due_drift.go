package functions

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

type clSearchResult struct {
	StartAt    int       `json:"startAt"`
	MaxResults int       `json:"maxResults"`
	Total      int       `json:"total"`
	Issues     []clIssue `json:"issues"`
}

type clIssue struct {
	Key       string             `json:"key"`
	Fields    models.IssueFields `json:"fields"`
	Changelog clChangelog        `json:"changelog"`
}

type clChangelog struct {
	Total     int         `json:"total"`
	Histories []clHistory `json:"histories"`
}

type clHistory struct {
	Author  *models.User `json:"author"`
	Created string       `json:"created"`
	Items   []clItem     `json:"items"`
}

type clItem struct {
	Field      string `json:"field"`
	From       string `json:"from"`
	FromString string `json:"fromString"`
	To         string `json:"to"`
	ToString   string `json:"toString"`
}

type driftEntry struct {
	key         string
	issueType   string
	summary     string
	author      string
	changes     []dateStep
	changeCount int
	totalDrift  int
	firstDue    string
	lastDue     string
}

type dateStep struct {
	fromDate string
	toDate   string
}

func RunDueDrift(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	projects := strings.Split(params["project"], ",")
	for i := range projects {
		projects[i] = strings.TrimSpace(projects[i])
	}

	var projectFilter string
	if len(projects) == 1 {
		projectFilter = fmt.Sprintf(`project = "%s"`, projects[0])
	} else {
		qp := make([]string, len(projects))
		for i, p := range projects {
			qp[i] = fmt.Sprintf("%q", p)
		}
		projectFilter = fmt.Sprintf(`project IN (%s)`, strings.Join(qp, ","))
	}

	period := params["period"]
	var days int
	switch period {
	case "Месяц":
		days = 30
	case "Полгода":
		days = 180
	default:
		days = 90
	}
	cutoff := time.Now().AddDate(0, 0, -days)

	jql := fmt.Sprintf(`%s AND issuetype IN ("История","Задача","Ошибка") AND updated >= -%dd`,
		projectFilter, days)
	fields := "key,summary,status,duedate,issuetype,assignee,creator"

	var allIssues []clIssue
	startAt := 0
	for {
		body, err := jira.DoSearchExpand(cfg, jql, fields, "changelog", startAt)
		if err != nil {
			return fmt.Errorf("ошибка загрузки задач: %v", err)
		}
		var result clSearchResult
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("ошибка парсинга: %v", err)
		}
		allIssues = append(allIssues, result.Issues...)
		out.SendProgress(len(allIssues), result.Total)
		if startAt+result.MaxResults >= result.Total {
			break
		}
		startAt += result.MaxResults
	}

	var drifts []driftEntry
	for _, issue := range allIssues {
		var steps []dateStep
		for _, h := range issue.Changelog.Histories {
			for _, item := range h.Items {
				if item.Field != "duedate" {
					continue
				}
				created := parseJiraTime(h.Created)
				if created.Before(cutoff) {
					continue
				}
				from := parseDueValue(item.FromString, item.From)
				to := parseDueValue(item.ToString, item.To)
				if from == "" && to == "" {
					continue
				}
				steps = append(steps, dateStep{fromDate: from, toDate: to})
			}
		}
		if len(steps) == 0 {
			continue
		}

		firstDue := steps[0].fromDate
		if firstDue == "" {
			firstDue = steps[0].toDate
		}
		lastDue := steps[len(steps)-1].toDate
		if lastDue == "" {
			lastDue = steps[len(steps)-1].fromDate
		}

		totalDrift := 0
		if firstDue != "" && lastDue != "" {
			t1, e1 := time.Parse("2006-01-02", firstDue)
			t2, e2 := time.Parse("2006-01-02", lastDue)
			if e1 == nil && e2 == nil {
				totalDrift = int(t2.Sub(t1).Hours() / 24)
			}
		}

		author := ""
		if issue.Fields.Assignee != nil {
			author = issue.Fields.Assignee.DisplayName
		} else if issue.Fields.Creator != nil {
			author = issue.Fields.Creator.DisplayName
		}

		drifts = append(drifts, driftEntry{
			key:         issue.Key,
			issueType:   issue.Fields.IssueType.Name,
			summary:     issue.Fields.Summary,
			author:      author,
			changes:     steps,
			changeCount: len(steps),
			totalDrift:  totalDrift,
			firstDue:    firstDue,
			lastDue:     lastDue,
		})
	}

	sort.Slice(drifts, func(i, j int) bool {
		ai := int(math.Abs(float64(drifts[i].totalDrift)))
		aj := int(math.Abs(float64(drifts[j].totalDrift)))
		if ai != aj {
			return ai > aj
		}
		return drifts[i].changeCount > drifts[j].changeCount
	})

	out.Printf("Загружено задач: %d | С переносами сроков: %d", len(allIssues), len(drifts))

	if len(drifts) == 0 {
		out.Printf("Задач с изменениями сроков не найдено.")
		return nil
	}

	headers := []string{"Ключ", "Тип", "Автор", "Изм.", "Исх. срок", "Тек. срок", "Сдвиг", "Диаграмма", "Название"}
	rows := make([][]string, 0, len(drifts))
	for _, d := range drifts {
		diagram := buildDriftDiagram(d.changes)
		driftStr := fmt.Sprintf("%+d дн", d.totalDrift)

		rows = append(rows, []string{
			d.key,
			d.issueType,
			d.author,
			fmt.Sprintf("%d", d.changeCount),
			fmtDriftDate(d.firstDue),
			fmtDriftDate(d.lastDue),
			driftStr,
			diagram,
			d.summary,
		})
	}
	out.SendTable(headers, rows)
	return nil
}

func parseJiraTime(s string) time.Time {
	formats := []string{
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05.000-07:00",
		"2006-01-02T15:04:05.000Z",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseDueValue(primary, fallback string) string {
	for _, s := range []string{primary, fallback} {
		if s == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", s); err == nil {
			return s
		}
		if t, err := time.Parse("02/Jan/06", s); err == nil {
			return t.Format("2006-01-02")
		}
		if t, err := time.Parse("02.01.2006", s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

func buildDriftDiagram(steps []dateStep) string {
	var dates []string
	if len(steps) > 0 && steps[0].fromDate != "" {
		dates = append(dates, fmtDriftDate(steps[0].fromDate))
	}
	for _, s := range steps {
		if s.toDate == "" {
			continue
		}
		formatted := fmtDriftDate(s.toDate)
		if len(dates) > 0 && dates[len(dates)-1] == formatted {
			continue
		}
		dates = append(dates, formatted)
	}
	return strings.Join(dates, " → ")
}

func fmtDriftDate(d string) string {
	if d == "" {
		return "—"
	}
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.Format("02.01.06")
}
