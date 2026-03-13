package functions

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

func RunOrphans(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	project := params["project"]
	jql := fmt.Sprintf(`project = "%s" AND issuetype = "Задача"`, project)

	var allIssues []models.Issue
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

	out.Printf("Проект: %s | Всего задач: %d | Без привязки к Историям: %d\n", project, len(allIssues), len(orphans))

	if len(orphans) == 0 {
		out.Printf("Все задачи привязаны к Историям.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "АВТОР\tKEY\tСТАТУС\tНАЗВАНИЕ")
	fmt.Fprintln(w, "-----\t---\t------\t--------")
	for _, issue := range orphans {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", jira.FormatAuthor(issue), issue.Key, issue.Fields.Status.Name, issue.Fields.Summary)
	}
	w.Flush()

	return nil
}
