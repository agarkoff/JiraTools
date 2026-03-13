package functions

import (
	"fmt"
	"strings"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

func RunSetEpic(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	epicKey := params["epic_key"]
	epicField := params["epic_field"]
	if epicField == "" {
		epicField = "customfield_10109"
	}

	// Parse task keys from textarea (one per line)
	var issueKeys []string
	for _, line := range strings.Split(params["task_keys"], "\n") {
		key := strings.TrimSpace(line)
		if key != "" {
			issueKeys = append(issueKeys, key)
		}
	}

	if len(issueKeys) == 0 {
		return fmt.Errorf("список задач пуст")
	}

	out.Printf("Эпик: %s | Задач: %d | Поле: %s", epicKey, len(issueKeys), epicField)

	set := 0
	errors := 0
	for _, key := range issueKeys {
		payload := map[string]interface{}{
			"fields": map[string]interface{}{
				epicField: epicKey,
			},
		}
		if err := jira.UpdateIssue(cfg, key, payload); err != nil {
			out.Printf("[ОШИБКА] установка эпика у %s: %v", key, err)
			errors++
		} else {
			set++
			out.SendProgress(set, len(issueKeys))
		}
	}
	out.Printf("Готово: %d успешно, %d ошибок", set, errors)

	return nil
}
