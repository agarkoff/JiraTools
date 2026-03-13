package functions

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

type userInfo struct {
	displayName string
	issues      []models.Issue
	totalTime   int
}

func RunWorkload(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	if len(cfg.Users) == 0 {
		return fmt.Errorf("в конфиге не указаны пользователи (поле \"users\")")
	}

	projects := strings.Split(params["project"], ",")
	for i := range projects {
		projects[i] = strings.TrimSpace(projects[i])
	}
	period := params["period"]
	if period == "" {
		period = "all"
	}

	// Формируем список пользователей для JQL
	quoted := make([]string, len(cfg.Users))
	for i, u := range cfg.Users {
		quoted[i] = fmt.Sprintf("%q", u)
	}

	// Формируем условие по проектам
	var projectFilter string
	if len(projects) == 1 {
		projectFilter = fmt.Sprintf(`project = "%s"`, projects[0])
	} else {
		quotedProjects := make([]string, len(projects))
		for i, p := range projects {
			quotedProjects[i] = fmt.Sprintf("%q", p)
		}
		projectFilter = fmt.Sprintf(`project IN (%s)`, strings.Join(quotedProjects, ","))
	}

	baseFilter := fmt.Sprintf(`%s AND issuetype IN ("Задача","Ошибка") AND status NOT IN ("Готово","Отклонено","Блокировано","Ревью")`, projectFilter)
	jql := fmt.Sprintf(`%s AND assignee IN (%s)`, baseFilter, strings.Join(quoted, ","))

	// Определяем период
	periodLabel := "все задачи"
	now := time.Now()
	switch period {
	case "week":
		weekday := now.Weekday()
		if weekday == time.Sunday {
			weekday = 7
		}
		monday := now.AddDate(0, 0, -int(weekday-time.Monday))
		sunday := monday.AddDate(0, 0, 6)
		datePart := fmt.Sprintf(` AND duedate >= "%s" AND duedate <= "%s"`, monday.Format("2006-01-02"), sunday.Format("2006-01-02"))
		jql += datePart
		periodLabel = fmt.Sprintf("неделя (%s — %s)", monday.Format("02.01"), sunday.Format("02.01"))
	case "month":
		firstDay := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		lastDay := firstDay.AddDate(0, 1, -1)
		datePart := fmt.Sprintf(` AND duedate >= "%s" AND duedate <= "%s"`, firstDay.Format("2006-01-02"), lastDay.Format("2006-01-02"))
		jql += datePart
		periodLabel = fmt.Sprintf("месяц (%s)", now.Format("01.2006"))
	case "all":
		// без фильтрации
	default:
		return fmt.Errorf("неизвестный период %q (допустимые: all, week, month)", period)
	}

	// Загружаем все задачи
	fields := "key,summary,status,assignee,timetracking,duedate"
	var allIssues []models.Issue
	startAt := 0
	for {
		result, err := jira.SearchIssues(cfg, jql, fields, startAt)
		if err != nil {
			return fmt.Errorf("Ошибка: %v", err)
		}
		allIssues = append(allIssues, result.Issues...)
		out.SendProgress(len(allIssues), result.Total)
		if startAt+result.MaxResults >= result.Total {
			break
		}
		startAt += result.MaxResults
	}

	// Группируем по assignee (по Name — логину)
	byUser := make(map[string]*userInfo)
	for _, issue := range allIssues {
		if issue.Fields.Assignee == nil {
			continue
		}
		login := issue.Fields.Assignee.Name
		if _, ok := byUser[login]; !ok {
			byUser[login] = &userInfo{
				displayName: issue.Fields.Assignee.DisplayName,
			}
		}
		est := 0
		if issue.Fields.TimeTracking != nil {
			est = issue.Fields.TimeTracking.OriginalEstimateSeconds
		}
		byUser[login].issues = append(byUser[login].issues, issue)
		byUser[login].totalTime += est
	}

	// Сортируем пользователей по displayName
	logins := make([]string, 0, len(byUser))
	for login := range byUser {
		logins = append(logins, login)
	}
	sort.Slice(logins, func(i, j int) bool {
		return byUser[logins[i]].displayName < byUser[logins[j]].displayName
	})

	// Заголовок
	out.Printf("Проект: %s | Период: %s | Пользователей: %d", strings.Join(projects, ", "), periodLabel, len(cfg.Users))

	// Детали по каждому пользователю
	for _, login := range logins {
		info := byUser[login]
		shortName := jira.FormatDisplayName(info.displayName)
		out.Printf("\n--- %s ---", shortName)

		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "KEY\tСТАТУС\tОЦЕНКА\tСРОК\tНАЗВАНИЕ")
		fmt.Fprintln(w, "---\t------\t------\t----\t--------")
		for _, issue := range info.issues {
			est := 0
			if issue.Fields.TimeTracking != nil {
				est = issue.Fields.TimeTracking.OriginalEstimateSeconds
			}
			duedate := issue.Fields.DueDate
			if duedate == "" {
				duedate = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				issue.Key,
				issue.Fields.Status.Name,
				jira.FormatHours(est),
				duedate,
				issue.Fields.Summary,
			)
		}
		w.Flush()
		out.Printf("Итого: %d задач(и), %s", len(info.issues), jira.FormatHours(info.totalTime))
	}

	// Показать пользователей без задач
	for _, u := range cfg.Users {
		if _, ok := byUser[u]; !ok {
			out.Printf("\n--- %s ---\nНет задач", u)
		}
	}

	// Сводка
	out.Printf("\n=== СВОДКА ===")
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ПОЛЬЗОВАТЕЛЬ\tЗАДАЧ\tОЦЕНКА")
	fmt.Fprintln(w, "------------\t-----\t------")

	// В сводке выводим всех пользователей из конфига
	allLogins := make([]string, len(cfg.Users))
	copy(allLogins, cfg.Users)
	sort.Slice(allLogins, func(i, j int) bool {
		nameI := allLogins[i]
		nameJ := allLogins[j]
		if info, ok := byUser[nameI]; ok {
			nameI = info.displayName
		}
		if info, ok := byUser[nameJ]; ok {
			nameJ = info.displayName
		}
		return nameI < nameJ
	})

	for _, login := range allLogins {
		info, ok := byUser[login]
		if ok {
			shortName := jira.FormatDisplayName(info.displayName)
			fmt.Fprintf(w, "%s\t%d\t%s\n", shortName, len(info.issues), jira.FormatHours(info.totalTime))
		} else {
			fmt.Fprintf(w, "%s\t0\t-\n", login)
		}
	}
	w.Flush()

	return nil
}
