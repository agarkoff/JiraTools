package functions

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

func RunChurn(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	project := params["project"]
	repoPath := params["repo_path"]
	if repoPath == "" {
		repoPath = "."
	}
	limit := 20
	if v, err := strconv.Atoi(params["limit"]); err == nil && v > 0 {
		limit = v
	}

	pattern := regexp.MustCompile(fmt.Sprintf(`(%s-\d+)`, regexp.QuoteMeta(project)))

	cmd := exec.Command("git", "log", "--numstat", "--pretty=format:COMMIT:%s")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ошибка выполнения git log в %s: %v", repoPath, err)
	}

	type taskChurn struct {
		key     string
		added   int
		deleted int
		commits int
	}

	churnMap := make(map[string]*taskChurn)
	var currentKeys []string

	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "COMMIT:") {
			matches := pattern.FindAllString(line[7:], -1)
			seen := make(map[string]bool)
			currentKeys = nil
			for _, m := range matches {
				if !seen[m] {
					seen[m] = true
					currentKeys = append(currentKeys, m)
				}
			}
			for _, key := range currentKeys {
				if _, ok := churnMap[key]; !ok {
					churnMap[key] = &taskChurn{key: key}
				}
				churnMap[key].commits++
			}
		} else if line != "" {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) == 3 && parts[0] != "-" {
				added, _ := strconv.Atoi(parts[0])
				deleted, _ := strconv.Atoi(parts[1])
				for _, key := range currentKeys {
					churnMap[key].added += added
					churnMap[key].deleted += deleted
				}
			}
		}
	}

	var tasks []*taskChurn
	for _, t := range churnMap {
		tasks = append(tasks, t)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return (tasks[i].added + tasks[i].deleted) > (tasks[j].added + tasks[j].deleted)
	})

	if limit > 0 && len(tasks) > limit {
		tasks = tasks[:limit]
	}

	// Load task summaries from Jira
	summaries := make(map[string]string)
	if len(tasks) > 0 {
		keys := make([]string, len(tasks))
		for i, t := range tasks {
			keys[i] = t.key
		}
		for i := 0; i < len(keys); i += 50 {
			end := i + 50
			if end > len(keys) {
				end = len(keys)
			}
			jql := fmt.Sprintf("key in (%s)", strings.Join(keys[i:end], ","))
			result, err := jira.SearchIssues(cfg, jql, "key,summary", 0)
			if err == nil {
				for _, issue := range result.Issues {
					summaries[issue.Key] = issue.Fields.Summary
				}
			}
		}
	}

	out.Printf("Проект: %s | Репозиторий: %s | Задач с изменениями: %d", project, repoPath, len(churnMap))

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tКОММИТЫ\t+ДОБАВЛЕНО\t-УДАЛЕНО\tИТОГО\tНАЗВАНИЕ")
	fmt.Fprintln(w, "---\t-------\t----------\t--------\t-----\t--------")
	for _, t := range tasks {
		summary := summaries[t.key]
		if summary == "" {
			summary = "-"
		}
		fmt.Fprintf(w, "%s\t%d\t+%d\t-%d\t%d\t%s\n",
			t.key, t.commits, t.added, t.deleted, t.added+t.deleted, summary)
	}
	w.Flush()

	return nil
}
