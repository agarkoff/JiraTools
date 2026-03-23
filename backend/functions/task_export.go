package functions

import (
	"fmt"
	"strings"
	"time"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

func RunTaskExport(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	projects := strings.Split(params["project"], ",")
	for i := range projects {
		projects[i] = strings.TrimSpace(projects[i])
	}
	refresh := params["refresh"] == "true"
	db := out.DB()

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

	jql := projectFilter
	if !refresh {
		allCfg, _ := models.GetConfig(db)
		if lastSync := allCfg["task_cache_sync"]; lastSync != "" {
			jql += fmt.Sprintf(` AND created >= "%s"`, lastSync)
		}
	}
	jql += " ORDER BY key ASC"

	fields := "key,summary,description,issuetype,status"
	var fetched []models.Issue
	startAt := 0
	for {
		result, err := jira.SearchIssues(cfg, jql, fields, startAt)
		if err != nil {
			return fmt.Errorf("ошибка загрузки: %v", err)
		}
		fetched = append(fetched, result.Issues...)
		out.SendProgress(len(fetched), result.Total)
		if startAt+result.MaxResults >= result.Total {
			break
		}
		startAt += result.MaxResults
	}

	// Save to DB
	for _, issue := range fetched {
		proj := strings.SplitN(issue.Key, "-", 2)[0]
		models.UpsertTaskCache(db, issue.Key, proj,
			issue.Fields.IssueType.Name,
			issue.Fields.Summary,
			issue.Fields.Description,
			issue.Fields.Status.Name)
	}

	models.SetConfig(db, "task_cache_sync", time.Now().Format("2006-01-02"))

	// Load all cached tasks
	tasks, err := models.GetCachedTasks(db, projects)
	if err != nil {
		return fmt.Errorf("ошибка чтения кеша: %v", err)
	}

	out.Printf("Загружено с сервера: %d | Всего в кеше: %d", len(fetched), len(tasks))

	if len(tasks) == 0 {
		out.Printf("Задач не найдено.")
		return nil
	}

	// Generate text file
	var sb strings.Builder
	for _, t := range tasks {
		fmt.Fprintf(&sb, "[%s] %s | %s\n", t.Key, t.IssueType, t.Status)
		sb.WriteString(t.Summary)
		sb.WriteString("\n")
		if t.Description != "" {
			sb.WriteString("\n")
			sb.WriteString(t.Description)
			sb.WriteString("\n")
		}
		sb.WriteString("\n---\n\n")
	}

	filename := fmt.Sprintf("tasks_%s.txt", strings.Join(projects, "_"))
	out.SendFile(filename, sb.String())

	return nil
}
