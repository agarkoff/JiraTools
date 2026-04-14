package functions

import (
	"fmt"
	"sort"
	"time"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

func RunWorklogs(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	if len(cfg.Users) == 0 {
		return fmt.Errorf("в конфиге не указаны пользователи (поле \"users\")")
	}

	period := params["period"]
	now := time.Now()

	// Determine Monday of the target week
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, -(weekday - 1))
	if period == "Прошлая неделя" {
		monday = monday.AddDate(0, 0, -7)
	}

	// Mon-Fri
	days := make([]time.Time, 5)
	for i := 0; i < 5; i++ {
		days[i] = monday.AddDate(0, 0, i)
	}

	fromStr := days[0].Format("2006-01-02")
	toStr := days[4].Format("2006-01-02")

	dayNames := []string{"Пн", "Вт", "Ср", "Чт", "Пт"}
	dayHeaders := make([]string, 5)
	dayKeys := make([]string, 5)
	for i, d := range days {
		dayHeaders[i] = fmt.Sprintf("%s %s", dayNames[i], d.Format("02.01"))
		dayKeys[i] = d.Format("2006-01-02")
	}

	out.Printf("Период: %s — %s", fromStr, toStr)

	// Collect per-user data
	type taskRow struct {
		key     string
		summary string
		perDay  [5]int
		total   int
	}
	type userData struct {
		login       string
		displayName string
		tasks       []taskRow
		dayTotals   [5]int
		total       int
	}
	var users []userData

	for idx, login := range cfg.Users {
		out.SendProgress(idx, len(cfg.Users))

		jql := fmt.Sprintf(`worklogAuthor = "%s" AND worklogDate >= "%s" AND worklogDate <= "%s"`,
			login, fromStr, toStr)

		var issues []models.Issue
		startAt := 0
		for {
			result, err := jira.SearchIssues(cfg, jql, "key,summary,status", startAt)
			if err != nil {
				out.Printf("[ОШИБКА] %s: %v", login, err)
				break
			}
			issues = append(issues, result.Issues...)
			if startAt+result.MaxResults >= result.Total {
				break
			}
			startAt += result.MaxResults
		}

		ud := userData{login: login, displayName: login}

		for _, issue := range issues {
			wls, err := jira.FetchIssueWorklogs(cfg, issue.Key)
			if err != nil {
				continue
			}

			var row taskRow
			row.key = issue.Key
			row.summary = issue.Fields.Summary

			for _, wl := range wls {
				if wl.Author == nil || wl.Author.Name != login {
					continue
				}
				if wl.Started == "" {
					continue
				}
				t, err := time.Parse("2006-01-02T15:04:05.000-0700", wl.Started)
				if err != nil {
					continue
				}
				d := t.Format("2006-01-02")
				for i, dk := range dayKeys {
					if d == dk {
						row.perDay[i] += wl.TimeSpentSeconds
						row.total += wl.TimeSpentSeconds
						if ud.displayName == login && wl.Author.DisplayName != "" {
							ud.displayName = wl.Author.DisplayName
						}
						break
					}
				}
			}

			if row.total > 0 {
				ud.tasks = append(ud.tasks, row)
				for i := 0; i < 5; i++ {
					ud.dayTotals[i] += row.perDay[i]
				}
				ud.total += row.total
			}
		}

		sort.Slice(ud.tasks, func(i, j int) bool {
			return ud.tasks[i].total > ud.tasks[j].total
		})

		users = append(users, ud)
	}

	out.SendProgress(len(cfg.Users), len(cfg.Users))

	// Summary table: user × days
	summaryHeaders := append([]string{"Пользователь"}, dayHeaders...)
	summaryHeaders = append(summaryHeaders, "Итого")
	summaryRows := make([][]string, 0, len(users)+1)
	var grandDayTotals [5]int
	grandTotal := 0

	for _, ud := range users {
		name := jira.FormatDisplayName(ud.displayName)
		if name == "" {
			name = ud.login
		}
		row := []string{name}
		for i := 0; i < 5; i++ {
			row = append(row, formatDuration(ud.dayTotals[i]))
			grandDayTotals[i] += ud.dayTotals[i]
		}
		row = append(row, formatDuration(ud.total))
		grandTotal += ud.total
		summaryRows = append(summaryRows, row)
	}

	totalsRow := []string{"Итого"}
	for i := 0; i < 5; i++ {
		totalsRow = append(totalsRow, formatDuration(grandDayTotals[i]))
	}
	totalsRow = append(totalsRow, formatDuration(grandTotal))
	summaryRows = append(summaryRows, totalsRow)

	out.SendTable(summaryHeaders, summaryRows)

	// Per-user detail tables
	for _, ud := range users {
		if len(ud.tasks) == 0 {
			continue
		}
		name := jira.FormatDisplayName(ud.displayName)
		if name == "" {
			name = ud.login
		}

		headers := append([]string{"Задача"}, dayHeaders...)
		headers = append(headers, "Итого", "Название")

		rows := make([][]string, 0, len(ud.tasks)+1)
		for _, t := range ud.tasks {
			row := []string{t.key}
			for i := 0; i < 5; i++ {
				row = append(row, formatDuration(t.perDay[i]))
			}
			row = append(row, formatDuration(t.total), t.summary)
			rows = append(rows, row)
		}

		// Totals row
		totRow := []string{"Итого"}
		for i := 0; i < 5; i++ {
			totRow = append(totRow, formatDuration(ud.dayTotals[i]))
		}
		totRow = append(totRow, formatDuration(ud.total), "")
		rows = append(rows, totRow)

		out.SendGroupedTable(
			fmt.Sprintf("%s — %s", name, formatDuration(ud.total)),
			"user", headers, rows,
		)
	}

	return nil
}

func formatDuration(seconds int) string {
	if seconds == 0 {
		return "-"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if m == 0 {
		return fmt.Sprintf("%dч", h)
	}
	if h == 0 {
		return fmt.Sprintf("%dм", m)
	}
	return fmt.Sprintf("%dч %dм", h, m)
}
