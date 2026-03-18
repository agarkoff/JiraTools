package functions

import (
	"fmt"
	"sort"
	"strings"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

func RunOrphans(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	projects := strings.Split(params["project"], ",")
	for i := range projects {
		projects[i] = strings.TrimSpace(projects[i])
	}

	var allIssues []models.Issue
	for _, project := range projects {
		jql := fmt.Sprintf(`project = "%s" AND issuetype = "Задача"`, project)
		startAt := 0
		for {
			result, err := jira.SearchIssuesDefault(cfg, jql, startAt)
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
	}

	var orphans []models.Issue
	for _, issue := range allIssues {
		if !jira.IsLinkedToStory(issue) {
			orphans = append(orphans, issue)
		}
	}

	sort.Slice(orphans, func(i, j int) bool {
		a := ""
		b := ""
		if orphans[i].Fields.Creator != nil {
			a = orphans[i].Fields.Creator.DisplayName
		}
		if orphans[j].Fields.Creator != nil {
			b = orphans[j].Fields.Creator.DisplayName
		}
		return a < b
	})

	out.Printf("Проекты: %s | Всего задач: %d | Без привязки к Историям: %d", strings.Join(projects, ", "), len(allIssues), len(orphans))

	if len(orphans) == 0 {
		out.Printf("Все задачи привязаны к Историям.")
		return nil
	}

	headers := []string{"Автор", "Key", "Статус", "Название"}
	rows := make([][]string, 0, len(orphans))
	for _, issue := range orphans {
		rows = append(rows, []string{
			jira.FormatAuthor(issue),
			issue.Key,
			issue.Fields.Status.Name,
			issue.Fields.Summary,
		})
	}
	out.SendTable(headers, rows)

	return nil
}
