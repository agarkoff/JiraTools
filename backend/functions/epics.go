package functions

import (
	"encoding/json"
	"fmt"
	"sort"
	"text/tabwriter"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

type epicTaskInfo struct {
	key       string
	summary   string
	status    string
	epicKey   string
	epicName  string
	viaParent bool
}

func RunEpics(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	project := params["project"]
	epicField := params["epic_field"]
	if epicField == "" {
		epicField = "customfield_10109"
	}
	removeEpic := params["remove_epic"] == "true"

	// 1. Load all epics for name lookup
	epicNames := make(map[string]string)
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
	out.Printf("Загружено эпиков: %d", len(epicNames))

	// 2. Load all tasks
	jql := fmt.Sprintf(`project = "%s" AND issuetype = "Задача"`, project)
	fields := "key,summary,status,parent"
	if epicField != "" {
		fields += "," + epicField
	}

	var tasks []epicTaskInfo
	startAt = 0
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
			t := epicTaskInfo{key: issue.Key}

			if raw, ok := issue.Fields["summary"]; ok {
				json.Unmarshal(raw, &t.summary)
			}
			if raw, ok := issue.Fields["status"]; ok {
				var status models.Status
				json.Unmarshal(raw, &status)
				t.status = status.Name
			}

			// Check parent for Epic type
			if raw, ok := issue.Fields["parent"]; ok && string(raw) != "null" {
				var parent models.ParentRef
				json.Unmarshal(raw, &parent)
				if parent.Fields.IssueType.Name == "Эпик" || parent.Fields.IssueType.Name == "Epic" {
					t.epicKey = parent.Key
					t.viaParent = true
					if name, ok := epicNames[t.epicKey]; ok {
						t.epicName = name
					}
				}
			}

			// Check custom Epic Link field
			if epicField != "" && t.epicKey == "" {
				if raw, ok := issue.Fields[epicField]; ok && string(raw) != "null" && len(raw) > 0 {
					var ek string
					if err := json.Unmarshal(raw, &ek); err == nil && ek != "" {
						t.epicKey = ek
						if name, ok := epicNames[ek]; ok {
							t.epicName = name
						}
					}
				}
			}

			tasks = append(tasks, t)
		}

		out.SendProgress(len(tasks), result.Total)
		if result.StartAt+result.MaxResults >= result.Total {
			break
		}
		startAt += result.MaxResults
	}

	// Count
	withEpic := 0
	for _, t := range tasks {
		if t.epicKey != "" {
			withEpic++
		}
	}

	out.Printf("Проект: %s | Всего задач: %d | С эпиком: %d | Без эпика: %d",
		project, len(tasks), withEpic, len(tasks)-withEpic)

	// Sort by issue number
	sort.Slice(tasks, func(i, j int) bool {
		return jira.IssueNum(tasks[i].key) < jira.IssueNum(tasks[j].key)
	})

	// Output table (only tasks with epic)
	if withEpic > 0 {
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "KEY\tСТАТУС\tЭПИК\tНАЗВАНИЕ ЭПИКА\tНАЗВАНИЕ ЗАДАЧИ")
		fmt.Fprintln(w, "---\t------\t----\t--------------\t---------------")
		for _, t := range tasks {
			if t.epicKey == "" {
				continue
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				t.key, t.status, t.epicKey, t.epicName, t.summary)
		}
		w.Flush()
	} else {
		out.Printf("Задач с эпиком не найдено.")
	}

	// Remove epics
	if removeEpic && withEpic > 0 {
		out.Printf("Удаление эпика у %d задач...", withEpic)
		removed := 0
		errors := 0
		for _, t := range tasks {
			if t.epicKey == "" {
				continue
			}
			var payload map[string]interface{}
			if t.viaParent {
				payload = map[string]interface{}{
					"fields": map[string]interface{}{"parent": nil},
				}
			} else if epicField != "" {
				payload = map[string]interface{}{
					"fields": map[string]interface{}{epicField: nil},
				}
			}
			if payload != nil {
				if err := jira.UpdateIssue(cfg, t.key, payload); err != nil {
					out.Printf("[ОШИБКА] удаление эпика у %s: %v", t.key, err)
					errors++
				} else {
					removed++
					out.SendProgress(removed, withEpic)
				}
			}
		}
		out.Printf("Удаление завершено: %d успешно, %d ошибок", removed, errors)
	}

	return nil
}
