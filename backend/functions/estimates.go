package functions

import (
	"fmt"
	"sort"
	"strings"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

type taskEstimate struct {
	issue         models.Issue
	estimate      float64 // hours (original estimate)
	spent         float64 // hours (total time on task)
	personalSpent float64 // hours (this user's logged time; = spent when not using worklogs)
	diff          float64 // personalSpent - estimate (positive = overrun)
	ratio         float64 // personalSpent / estimate (0 if no estimate)
}

type personEstimates struct {
	login       string
	displayName string
	tasks       []taskEstimate
}

func RunEstimates(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	if len(cfg.Users) == 0 {
		return fmt.Errorf("в конфиге не указаны пользователи (поле \"users\")")
	}

	projects := strings.Split(params["project"], ",")
	for i := range projects {
		projects[i] = strings.TrimSpace(projects[i])
	}
	useWorklogs := params["worklogs"] == "true"

	quoted := make([]string, len(cfg.Users))
	for i, u := range cfg.Users {
		quoted[i] = fmt.Sprintf("%q", u)
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

	userList := strings.Join(quoted, ",")
	var jql string
	var fields string
	if useWorklogs {
		jql = fmt.Sprintf(`%s AND issuetype IN ("Задача","Ошибка") AND (assignee IN (%s) OR worklogAuthor IN (%s)) ORDER BY key`,
			projectFilter, userList, userList)
		fields = "key,summary,status,assignee,timetracking,worklog"
	} else {
		jql = fmt.Sprintf(`%s AND issuetype IN ("Задача","Ошибка") AND assignee IN (%s) ORDER BY assignee`,
			projectFilter, userList)
		fields = "key,summary,status,assignee,timetracking"
	}

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

	// Set of configured users for quick lookup
	userSet := make(map[string]bool)
	for _, u := range cfg.Users {
		userSet[u] = true
	}

	byUser := make(map[string]*personEstimates)
	var allTasks []taskEstimate // task-level (deduplicated)

	if useWorklogs {
		// Fetch worklogs where inline data is incomplete
		needFetch := 0
		for _, issue := range allIssues {
			if wl := issue.Fields.Worklog; wl == nil || wl.Total > len(wl.Worklogs) {
				needFetch++
			}
		}
		if needFetch > 0 {
			out.Printf("Загрузка ворклогов: %d задач требуют дозагрузки...", needFetch)
		}

		for idx, issue := range allIssues {
			est := 0.0
			totalSpent := 0.0
			if issue.Fields.TimeTracking != nil {
				est = float64(issue.Fields.TimeTracking.OriginalEstimateSeconds) / 3600
				totalSpent = float64(issue.Fields.TimeTracking.TimeSpentSeconds) / 3600
			}

			worklogs := jira.GetCompleteWorklogs(issue, cfg)

			if needFetch > 0 {
				out.SendProgress(idx+1, len(allIssues))
			}

			// Sum per user
			userTime := make(map[string]float64)
			userDisplayName := make(map[string]string)
			for _, wl := range worklogs {
				if wl.Author == nil {
					continue
				}
				login := wl.Author.Name
				userTime[login] += float64(wl.TimeSpentSeconds) / 3600
				if _, ok := userDisplayName[login]; !ok {
					userDisplayName[login] = wl.Author.DisplayName
				}
			}

			// Add to per-user data
			for login, pTime := range userTime {
				if !userSet[login] {
					continue
				}

				if _, ok := byUser[login]; !ok {
					displayName := userDisplayName[login]
					if displayName == "" {
						displayName = login
					}
					byUser[login] = &personEstimates{
						login:       login,
						displayName: displayName,
					}
				}

				te := taskEstimate{
					issue:         issue,
					estimate:      est,
					spent:         totalSpent,
					personalSpent: pTime,
					diff:          pTime - est,
				}
				if est > 0 {
					te.ratio = pTime / est
				}
				byUser[login].tasks = append(byUser[login].tasks, te)
			}

			// Task-level entry for allTasks (total spent, deduplicated)
			tTask := taskEstimate{
				issue:         issue,
				estimate:      est,
				spent:         totalSpent,
				personalSpent: totalSpent,
				diff:          totalSpent - est,
			}
			if est > 0 {
				tTask.ratio = totalSpent / est
			}
			allTasks = append(allTasks, tTask)
		}
	} else {
		for _, issue := range allIssues {
			if issue.Fields.Assignee == nil {
				continue
			}
			login := issue.Fields.Assignee.Name
			if _, ok := byUser[login]; !ok {
				byUser[login] = &personEstimates{
					login:       login,
					displayName: issue.Fields.Assignee.DisplayName,
				}
			}

			est := 0.0
			spent := 0.0
			if issue.Fields.TimeTracking != nil {
				est = float64(issue.Fields.TimeTracking.OriginalEstimateSeconds) / 3600
				spent = float64(issue.Fields.TimeTracking.TimeSpentSeconds) / 3600
			}

			te := taskEstimate{
				issue:         issue,
				estimate:      est,
				spent:         spent,
				personalSpent: spent,
				diff:          spent - est,
			}
			if est > 0 {
				te.ratio = spent / est
			}

			byUser[login].tasks = append(byUser[login].tasks, te)
			allTasks = append(allTasks, te)
		}
	}

	// Sort users by displayName
	logins := make([]string, 0, len(byUser))
	for login := range byUser {
		logins = append(logins, login)
	}
	sort.Slice(logins, func(i, j int) bool {
		return byUser[logins[i]].displayName < byUser[logins[j]].displayName
	})

	// Console summary
	mode := ""
	if useWorklogs {
		mode = " (worklogs)"
	}
	out.Printf("Проекты: %s | Всего задач: %d | Пользователей: %d%s\n",
		strings.Join(projects, ", "), len(allTasks), len(logins), mode)

	spentLabel := "Затрачено (ч)"
	if useWorklogs {
		spentLabel = "Лично (ч)"
	}

	headers := []string{"Пользователь", "Задач", "С оценкой", "С логом", "Оценка (ч)", spentLabel, "Разница (ч)"}
	rows := make([][]string, 0, len(logins)+1)

	totalTasks := 0
	totalWithEst := 0
	totalWithSpent := 0
	totalEst := 0.0
	totalSpent := 0.0

	for _, login := range logins {
		pe := byUser[login]
		withEst := 0
		withSpent := 0
		sumEst := 0.0
		sumSpent := 0.0
		for _, t := range pe.tasks {
			if t.estimate > 0 {
				withEst++
			}
			if t.personalSpent > 0 {
				withSpent++
			}
			sumEst += t.estimate
			sumSpent += t.personalSpent
		}
		totalTasks += len(pe.tasks)
		totalWithEst += withEst
		totalWithSpent += withSpent
		totalEst += sumEst
		totalSpent += sumSpent
		rows = append(rows, []string{
			jira.FormatDisplayName(pe.displayName),
			fmt.Sprintf("%d", len(pe.tasks)),
			fmt.Sprintf("%d", withEst),
			fmt.Sprintf("%d", withSpent),
			fmt.Sprintf("%.1f", sumEst),
			fmt.Sprintf("%.1f", sumSpent),
			fmt.Sprintf("%.1f", sumSpent-sumEst),
		})
	}
	rows = append(rows, []string{
		"ИТОГО",
		fmt.Sprintf("%d", totalTasks),
		fmt.Sprintf("%d", totalWithEst),
		fmt.Sprintf("%d", totalWithSpent),
		fmt.Sprintf("%.1f", totalEst),
		fmt.Sprintf("%.1f", totalSpent),
		fmt.Sprintf("%.1f", totalSpent-totalEst),
	})
	out.SendTable(headers, rows)

	return nil
}
