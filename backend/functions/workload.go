package functions

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"jira-tools-web/calendar"
	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

type userInfo struct {
	displayName string
	issues      []models.Issue
	totalTime   int
}

// Gantt chart types
type ganttTask struct {
	Key           string  `json:"key"`
	Summary       string  `json:"summary"`
	Start         string  `json:"start"`
	End           string  `json:"end"`
	DueDate       string  `json:"due_date,omitempty"`
	EstimateHours float64 `json:"estimate_hours"`
	Overdue       bool    `json:"overdue"`
	Status        string  `json:"status"`
	PriorityID    string  `json:"priority_id"`
	PriorityName  string  `json:"priority_name"`
}

type ganttUser struct {
	Name       string      `json:"name"`
	Tasks      []ganttTask `json:"tasks"`
	Overloaded bool        `json:"overloaded"`
	TotalHours float64     `json:"total_hours"`
}

type ganttChartData struct {
	Users          []ganttUser `json:"users"`
	DateStart      string      `json:"date_start"`
	DateEnd        string      `json:"date_end"`
	Today          string      `json:"today"`
	NonWorkingDays []string    `json:"non_working_days"`
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
	case "Неделя", "week":
		weekday := now.Weekday()
		if weekday == time.Sunday {
			weekday = 7
		}
		monday := now.AddDate(0, 0, -int(weekday-time.Monday))
		sunday := monday.AddDate(0, 0, 6)
		datePart := fmt.Sprintf(` AND duedate >= "%s" AND duedate <= "%s"`, monday.Format("2006-01-02"), sunday.Format("2006-01-02"))
		jql += datePart
		periodLabel = fmt.Sprintf("неделя (%s — %s)", monday.Format("02.01"), sunday.Format("02.01"))
	case "Месяц", "month":
		firstDay := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		lastDay := firstDay.AddDate(0, 1, -1)
		datePart := fmt.Sprintf(` AND duedate >= "%s" AND duedate <= "%s"`, firstDay.Format("2006-01-02"), lastDay.Format("2006-01-02"))
		jql += datePart
		periodLabel = fmt.Sprintf("месяц (%s)", now.Format("01.2006"))
	default:
		// "Все", "all" — без фильтрации
	}

	// Загружаем все задачи
	fields := "key,summary,status,assignee,timetracking,duedate,priority"
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

	// Сводка (первой)
	summaryHeaders := []string{"Пользователь", "Задач", "Оценка"}
	summaryRows := make([][]string, 0, len(cfg.Users))

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
			summaryRows = append(summaryRows, []string{shortName, fmt.Sprintf("%d", len(info.issues)), jira.FormatHours(info.totalTime)})
		} else {
			summaryRows = append(summaryRows, []string{login, "0", "-"})
		}
	}
	out.SendTable(summaryHeaders, summaryRows)

	// --- Gantt chart ---
	var gUsers []ganttUser
	if len(logins) > 0 {
		today := time.Now().Truncate(24 * time.Hour)
		chartStart := today
		chartEnd := today

		for _, login := range logins {
			info := byUser[login]
			shortName := jira.FormatDisplayName(info.displayName)

			// Split tasks: with duedate and without
			type taskWithDue struct {
				issue   models.Issue
				dueDate time.Time
			}
			var withDue []taskWithDue
			var withoutDue []models.Issue

			for _, issue := range info.issues {
				if issue.Fields.DueDate != "" {
					if d, err := time.Parse("2006-01-02", issue.Fields.DueDate); err == nil {
						withDue = append(withDue, taskWithDue{issue, d})
					} else {
						withoutDue = append(withoutDue, issue)
					}
				} else {
					withoutDue = append(withoutDue, issue)
				}
			}

			// Sort by duedate first, then by priority (lower ID = higher priority)
			sort.Slice(withDue, func(i, j int) bool {
				if !withDue[i].dueDate.Equal(withDue[j].dueDate) {
					return withDue[i].dueDate.Before(withDue[j].dueDate)
				}
				pi, pj := "999", "999"
				if p := withDue[i].issue.Fields.Priority; p != nil {
					pi = p.ID
				}
				if p := withDue[j].issue.Fields.Priority; p != nil {
					pj = p.ID
				}
				return pi < pj
			})

			// Sort tasks without duedate by priority too
			sort.Slice(withoutDue, func(i, j int) bool {
				pi, pj := "999", "999"
				if p := withoutDue[i].Fields.Priority; p != nil {
					pi = p.ID
				}
				if p := withoutDue[j].Fields.Priority; p != nil {
					pj = p.ID
				}
				return pi < pj
			})

			// Schedule sequentially from today
			cursor := calendar.SkipToWorkDay(today)
			var gTasks []ganttTask
			overloaded := false
			totalHours := float64(0)

			scheduleTask := func(issue models.Issue, hasDue bool, dueDate time.Time) {
				estSec := 0
				if issue.Fields.TimeTracking != nil {
					estSec = issue.Fields.TimeTracking.OriginalEstimateSeconds
				}
				estHours := float64(estSec) / 3600.0
				totalHours += estHours

				workDays := 1
				if estSec > 0 {
					workDays = estSec / (8 * 3600)
					if estSec%(8*3600) > 0 {
						workDays++
					}
				}

				start := cursor
				end := calendar.AddWorkDays(cursor, workDays-1)
				cursor = calendar.AddWorkDays(end, 1)

				isOverdue := false
				dueDateStr := ""
				if hasDue {
					dueDateStr = dueDate.Format("2006-01-02")
					if end.After(dueDate) {
						isOverdue = true
						overloaded = true
					}
				}

				// Update chart bounds
				if start.Before(chartStart) {
					chartStart = start
				}
				if end.After(chartEnd) {
					chartEnd = end
				}
				if hasDue && dueDate.After(chartEnd) {
					chartEnd = dueDate
				}

				pID, pName := "", ""
				if p := issue.Fields.Priority; p != nil {
					pID = p.ID
					pName = p.Name
				}

				gTasks = append(gTasks, ganttTask{
					Key:           issue.Key,
					Summary:       issue.Fields.Summary,
					Start:         start.Format("2006-01-02"),
					End:           end.Format("2006-01-02"),
					DueDate:       dueDateStr,
					EstimateHours: estHours,
					Overdue:       isOverdue,
					Status:        issue.Fields.Status.Name,
					PriorityID:    pID,
					PriorityName:  pName,
				})
			}

			for _, td := range withDue {
				scheduleTask(td.issue, true, td.dueDate)
			}
			for _, issue := range withoutDue {
				scheduleTask(issue, false, time.Time{})
			}

			gUsers = append(gUsers, ganttUser{
				Name:       shortName,
				Tasks:      gTasks,
				Overloaded: overloaded,
				TotalHours: totalHours,
			})
		}

		// Add buffer to chart end
		chartEnd = chartEnd.AddDate(0, 0, 2)

		out.SendGantt(ganttChartData{
			Users:          gUsers,
			DateStart:      chartStart.Format("2006-01-02"),
			DateEnd:        chartEnd.Format("2006-01-02"),
			Today:          today.Format("2006-01-02"),
			NonWorkingDays: calendar.GetNonWorkingDays(chartStart, chartEnd),
		})
	}

	// --- Нарушения приоритетов ---
	// Ищем случаи когда важная задача просрочена, а менее важная у того же пользователя — нет.
	// Это значит что менее важные задачи «крадут» время у более важных.
	type violation struct {
		user     string
		hiKey    string
		hiPrio   string
		hiDue    string
		loKey    string
		loPrio   string
		loDue    string
	}
	var violations []violation

	for _, gu := range gUsers {
		if !gu.Overloaded {
			continue
		}
		// Собираем просроченные и непросроченные задачи с приоритетом
		var overdueTasks, okTasks []ganttTask
		for _, t := range gu.Tasks {
			if t.DueDate == "" || t.PriorityID == "" {
				continue
			}
			if t.Overdue {
				overdueTasks = append(overdueTasks, t)
			} else {
				okTasks = append(okTasks, t)
			}
		}

		// Для каждой просроченной важной задачи ищем непросроченную менее важную,
		// которая запланирована ДО дедлайна важной задачи (т.е. реально крадёт время)
		for _, hi := range overdueTasks {
			hiDue, err := time.Parse("2006-01-02", hi.DueDate)
			if err != nil {
				continue
			}
			for _, lo := range okTasks {
				if hi.PriorityID >= lo.PriorityID {
					continue
				}
				// Менее важная задача запланирована до дедлайна важной?
				loStart, err := time.Parse("2006-01-02", lo.Start)
				if err != nil {
					continue
				}
				if loStart.Before(hiDue) {
					violations = append(violations, violation{
						user:   gu.Name,
						hiKey:  hi.Key,
						hiPrio: hi.PriorityName,
						hiDue:  hi.DueDate,
						loKey:  lo.Key,
						loPrio: lo.PriorityName,
						loDue:  lo.DueDate,
					})
				}
			}
		}
	}

	if len(violations) > 0 {
		out.Printf("")
		out.Printf("Нарушения приоритетов: %d (важная задача просрочена, а менее важная — нет)", len(violations))
		vHeaders := []string{"Пользователь", "Просроченная", "Приоритет", "Срок", "Успевает", "Приоритет", "Срок"}
		vRows := make([][]string, 0, len(violations))
		for _, v := range violations {
			vRows = append(vRows, []string{v.user, v.hiKey, v.hiPrio, v.hiDue, v.loKey, v.loPrio, v.loDue})
		}
		out.SendTable(vHeaders, vRows)
	}

	// Детали по каждому пользователю (grouped)
	for _, login := range logins {
		info := byUser[login]
		shortName := jira.FormatDisplayName(info.displayName)

		headers := []string{"Key", "Статус", "Оценка", "Срок", "Название"}
		rows := make([][]string, 0, len(info.issues))
		for _, issue := range info.issues {
			est := 0
			if issue.Fields.TimeTracking != nil {
				est = issue.Fields.TimeTracking.OriginalEstimateSeconds
			}
			duedate := issue.Fields.DueDate
			if duedate == "" {
				duedate = "-"
			}
			rows = append(rows, []string{
				issue.Key,
				issue.Fields.Status.Name,
				jira.FormatHours(est),
				duedate,
				issue.Fields.Summary,
			})
		}
		out.SendGroupedTable(shortName, "user", headers, rows)
	}

	return nil
}
