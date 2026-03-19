package functions

import (
	"fmt"
	"sort"
	"strings"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

func RunDueMismatch(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
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

	// 1. Load all tasks/bugs with duedate
	skipDone := params["skip_done"] == "true"
	jql := fmt.Sprintf(`%s AND issuetype IN ("Задача","Ошибка") AND duedate IS NOT EMPTY`, projectFilter)
	if skipDone {
		jql += ` AND status NOT IN ("Готово","Отклонено")`
	}
	fields := "key,summary,status,duedate,parent,issuelinks,issuetype"
	var tasks []models.Issue
	startAt := 0
	for {
		result, err := jira.SearchIssues(cfg, jql, fields, startAt)
		if err != nil {
			return fmt.Errorf("ошибка загрузки задач: %v", err)
		}
		tasks = append(tasks, result.Issues...)
		out.SendProgress(len(tasks), result.Total)
		if startAt+result.MaxResults >= result.Total {
			break
		}
		startAt += result.MaxResults
	}

	// 2. Collect all story keys referenced by tasks
	storyKeys := make(map[string]bool)
	for _, task := range tasks {
		if p := task.Fields.Parent; p != nil && p.Fields.IssueType.Name == "История" {
			storyKeys[p.Key] = true
		}
		for _, link := range task.Fields.IssueLinks {
			if link.InwardIssue != nil && link.InwardIssue.Fields.IssueType.Name == "История" {
				storyKeys[link.InwardIssue.Key] = true
			}
			if link.OutwardIssue != nil && link.OutwardIssue.Fields.IssueType.Name == "История" {
				storyKeys[link.OutwardIssue.Key] = true
			}
		}
	}

	if len(storyKeys) == 0 {
		out.Printf("Задач со сроком: %d | Связанных историй не найдено", len(tasks))
		return nil
	}

	// 3. Load stories with duedate
	keys := make([]string, 0, len(storyKeys))
	for k := range storyKeys {
		keys = append(keys, k)
	}

	// Fetch stories in batches via JQL key IN (...)
	storyMap := make(map[string]models.Issue)
	batchSize := 50
	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}
		batch := keys[i:end]
		quoted := make([]string, len(batch))
		for j, k := range batch {
			quoted[j] = fmt.Sprintf("%q", k)
		}
		storyJQL := fmt.Sprintf("key IN (%s)", strings.Join(quoted, ","))
		startAt = 0
		for {
			result, err := jira.SearchIssues(cfg, storyJQL, "key,summary,status,duedate", startAt)
			if err != nil {
				return fmt.Errorf("ошибка загрузки историй: %v", err)
			}
			for _, s := range result.Issues {
				storyMap[s.Key] = s
			}
			if startAt+result.MaxResults >= result.Total {
				break
			}
			startAt += result.MaxResults
		}
	}

	// 4. Find mismatches: task duedate > story duedate
	type mismatch struct {
		taskKey     string
		taskSummary string
		taskDue     string
		taskStatus  string
		storyKey    string
		storySummary string
		storyDue    string
	}
	var mismatches []mismatch

	for _, task := range tasks {
		taskDue := task.Fields.DueDate
		if taskDue == "" {
			continue
		}

		// Find linked story
		var storyKey string
		if p := task.Fields.Parent; p != nil && p.Fields.IssueType.Name == "История" {
			storyKey = p.Key
		}
		if storyKey == "" {
			for _, link := range task.Fields.IssueLinks {
				if link.InwardIssue != nil && link.InwardIssue.Fields.IssueType.Name == "История" {
					storyKey = link.InwardIssue.Key
					break
				}
				if link.OutwardIssue != nil && link.OutwardIssue.Fields.IssueType.Name == "История" {
					storyKey = link.OutwardIssue.Key
					break
				}
			}
		}
		if storyKey == "" {
			continue
		}

		story, ok := storyMap[storyKey]
		if !ok || story.Fields.DueDate == "" {
			continue
		}

		if taskDue > story.Fields.DueDate {
			mismatches = append(mismatches, mismatch{
				taskKey:      task.Key,
				taskSummary:  task.Fields.Summary,
				taskDue:      taskDue,
				taskStatus:   task.Fields.Status.Name,
				storyKey:     story.Key,
				storySummary: story.Fields.Summary,
				storyDue:     story.Fields.DueDate,
			})
		}
	}

	sort.Slice(mismatches, func(i, j int) bool {
		return mismatches[i].storyKey < mismatches[j].storyKey
	})

	out.Printf("Задач со сроком: %d | Несоответствий: %d", len(tasks), len(mismatches))

	if len(mismatches) == 0 {
		out.Printf("Все сроки задач не превышают сроки историй.")
		return nil
	}

	headers := []string{"Задача", "Срок задачи", "Статус", "История", "Срок истории", "Название задачи"}
	rows := make([][]string, 0, len(mismatches))
	for _, m := range mismatches {
		rows = append(rows, []string{
			m.taskKey,
			m.taskDue,
			m.taskStatus,
			m.storyKey,
			m.storyDue,
			m.taskSummary,
		})
	}
	out.SendTable(headers, rows)

	return nil
}
