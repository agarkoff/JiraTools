package functions

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

type bugEpicInfo struct {
	key       string
	summary   string
	status    string
	epicKey   string
	epicName  string
	viaParent bool
	storyKey  string
}

func RunBugEpicCleanup(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	projects := strings.Split(params["project"], ",")
	for i := range projects {
		projects[i] = strings.TrimSpace(projects[i])
	}
	epicField := params["epic_field"]
	if epicField == "" {
		epicField = "customfield_10109"
	}
	removeEpic := params["remove_epic"] == "true"

	// 1. Load epic names for lookup
	epicNames := make(map[string]string)
	for _, project := range projects {
		epicJQL := fmt.Sprintf(`project = "%s" AND issuetype = "Epic"`, project)
		startAt := 0
		for {
			result, err := jira.SearchIssues(cfg, epicJQL, "key,summary", startAt)
			if err != nil {
				return fmt.Errorf("ошибка загрузки эпиков: %v", err)
			}
			for _, issue := range result.Issues {
				epicNames[issue.Key] = issue.Fields.Summary
			}
			if startAt+result.MaxResults >= result.Total {
				break
			}
			startAt += result.MaxResults
		}
	}
	out.Printf("Загружено эпиков: %d", len(epicNames))

	// 2. Load all bugs
	projectClauses := make([]string, len(projects))
	for i, p := range projects {
		projectClauses[i] = fmt.Sprintf(`project = "%s"`, p)
	}
	jql := fmt.Sprintf(`(%s) AND issuetype = "Ошибка"`, strings.Join(projectClauses, " OR "))
	fields := "key,summary,status,parent,issuelinks"
	if epicField != "" {
		fields += "," + epicField
	}

	var candidates []bugEpicInfo
	totalBugs := 0
	startAt := 0
	for {
		body, err := jira.DoSearch(cfg, jql, fields, startAt)
		if err != nil {
			return fmt.Errorf("ошибка: %v", err)
		}

		var result models.RawSearchResult
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("ошибка парсинга: %v", err)
		}

		for _, issue := range result.Issues {
			totalBugs++
			b := bugEpicInfo{key: issue.Key}

			if raw, ok := issue.Fields["summary"]; ok {
				json.Unmarshal(raw, &b.summary)
			}
			if raw, ok := issue.Fields["status"]; ok {
				var status models.Status
				json.Unmarshal(raw, &status)
				b.status = status.Name
			}

			// Check parent for Epic
			if raw, ok := issue.Fields["parent"]; ok && string(raw) != "null" {
				var parent models.ParentRef
				json.Unmarshal(raw, &parent)
				if parent.Fields.IssueType.Name == "Эпик" || parent.Fields.IssueType.Name == "Epic" {
					b.epicKey = parent.Key
					b.viaParent = true
					if name, ok := epicNames[b.epicKey]; ok {
						b.epicName = name
					}
				}
			}

			// Check custom Epic Link field
			if epicField != "" && b.epicKey == "" {
				if raw, ok := issue.Fields[epicField]; ok && string(raw) != "null" && len(raw) > 0 {
					var ek string
					if err := json.Unmarshal(raw, &ek); err == nil && ek != "" {
						b.epicKey = ek
						if name, ok := epicNames[ek]; ok {
							b.epicName = name
						}
					}
				}
			}

			// No epic — skip
			if b.epicKey == "" {
				continue
			}

			// Check issuelinks for a Story
			if raw, ok := issue.Fields["issuelinks"]; ok {
				var links []models.IssueLink
				json.Unmarshal(raw, &links)
				for _, link := range links {
					if link.InwardIssue != nil {
						t := link.InwardIssue.Fields.IssueType.Name
						if t == "История" || t == "Story" {
							b.storyKey = link.InwardIssue.Key
							break
						}
					}
					if link.OutwardIssue != nil {
						t := link.OutwardIssue.Fields.IssueType.Name
						if t == "История" || t == "Story" {
							b.storyKey = link.OutwardIssue.Key
							break
						}
					}
				}
			}

			if b.storyKey != "" {
				candidates = append(candidates, b)
			}
		}

		out.SendProgress(totalBugs, result.Total)
		if result.StartAt+result.MaxResults >= result.Total {
			break
		}
		startAt += result.MaxResults
	}

	out.Printf("Проекты: %s | Всего ошибок: %d | С эпиком и связью с историей: %d",
		strings.Join(projects, ", "), totalBugs, len(candidates))

	if len(candidates) == 0 {
		out.Printf("Ошибок с эпиком и связью с историей не найдено.")
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return jira.IssueNum(candidates[i].key) < jira.IssueNum(candidates[j].key)
	})

	headers := []string{"Ошибка", "Статус", "Эпик", "Название эпика", "История", "Название"}
	rows := make([][]string, 0, len(candidates))
	for _, b := range candidates {
		rows = append(rows, []string{b.key, b.status, b.epicKey, b.epicName, b.storyKey, b.summary})
	}
	out.SendTable(headers, rows)

	if removeEpic {
		out.Printf("Удаление эпика у %d ошибок...", len(candidates))
		removed := 0
		errors := 0
		for _, b := range candidates {
			var payload map[string]interface{}
			if b.viaParent {
				payload = map[string]interface{}{
					"fields": map[string]interface{}{"parent": nil},
				}
			} else if epicField != "" {
				payload = map[string]interface{}{
					"fields": map[string]interface{}{epicField: nil},
				}
			}
			if payload != nil {
				if err := jira.UpdateIssue(cfg, b.key, payload); err != nil {
					out.Printf("[ОШИБКА] %s: %v", b.key, err)
					errors++
				} else {
					removed++
					out.SendProgress(removed, len(candidates))
				}
			}
		}
		out.Printf("Удаление завершено: %d успешно, %d ошибок", removed, errors)
	}

	return nil
}
