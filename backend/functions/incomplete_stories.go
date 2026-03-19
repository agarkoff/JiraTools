package functions

import (
	"fmt"
	"sort"
	"strings"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

func RunIncompleteStories(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
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

	// 1. Load stories in "Готово" status
	jql := fmt.Sprintf(`%s AND issuetype = "История" AND status = "Готово"`, projectFilter)
	fields := "key,summary,status,issuelinks,parent"

	var stories []models.Issue
	startAt := 0
	for {
		result, err := jira.SearchIssues(cfg, jql, fields, startAt)
		if err != nil {
			return fmt.Errorf("ошибка загрузки историй: %v", err)
		}
		stories = append(stories, result.Issues...)
		out.SendProgress(len(stories), result.Total)
		if startAt+result.MaxResults >= result.Total {
			break
		}
		startAt += result.MaxResults
	}

	if len(stories) == 0 {
		out.Printf("Историй в статусе «Готово» не найдено.")
		return nil
	}

	// 2. Collect all child task keys from issue links
	// Children are linked via parent field or issuelinks (parentof/subtask)
	storyKeys := make(map[string]bool)
	for _, s := range stories {
		storyKeys[s.Key] = true
	}

	// 3. Load all tasks/bugs that have a parent story in our set
	// We search for tasks whose parent is one of our stories
	// Batch by story keys
	childMap := make(map[string][]models.Issue) // storyKey -> child issues

	allStoryKeys := make([]string, 0, len(stories))
	for _, s := range stories {
		allStoryKeys = append(allStoryKeys, s.Key)
	}

	batchSize := 50
	for i := 0; i < len(allStoryKeys); i += batchSize {
		end := i + batchSize
		if end > len(allStoryKeys) {
			end = len(allStoryKeys)
		}
		batch := allStoryKeys[i:end]
		quoted := make([]string, len(batch))
		for j, k := range batch {
			quoted[j] = fmt.Sprintf("%q", k)
		}

		// Search for tasks/bugs whose parent is one of the stories
		childJQL := fmt.Sprintf(`parent IN (%s) AND issuetype IN ("Задача","Ошибка")`, strings.Join(quoted, ","))
		startAt = 0
		for {
			result, err := jira.SearchIssues(cfg, childJQL, "key,summary,status,parent,issuetype", startAt)
			if err != nil {
				return fmt.Errorf("ошибка загрузки задач: %v", err)
			}
			for _, child := range result.Issues {
				if child.Fields.Parent != nil {
					childMap[child.Fields.Parent.Key] = append(childMap[child.Fields.Parent.Key], child)
				}
			}
			if startAt+result.MaxResults >= result.Total {
				break
			}
			startAt += result.MaxResults
		}
	}

	// Also check via issuelinks — any link to a task/bug counts as child
	addLinkedChild := func(storyKey string, linked *models.LinkedIssue) {
		if linked == nil {
			return
		}
		it := linked.Fields.IssueType.Name
		if it != "Задача" && it != "Ошибка" {
			return
		}
		childMap[storyKey] = append(childMap[storyKey], models.Issue{
			Key:    linked.Key,
			Fields: models.IssueFields{IssueType: linked.Fields.IssueType},
		})
	}

	for _, story := range stories {
		for _, link := range story.Fields.IssueLinks {
			addLinkedChild(story.Key, link.OutwardIssue)
			addLinkedChild(story.Key, link.InwardIssue)
		}
	}

	// 4. For link-based children we only have keys, need to fetch their statuses
	var linkChildKeys []string
	linkChildSet := make(map[string]bool)
	for _, children := range childMap {
		for _, c := range children {
			if c.Fields.Status.Name == "" && !linkChildSet[c.Key] {
				linkChildSet[c.Key] = true
				linkChildKeys = append(linkChildKeys, c.Key)
			}
		}
	}

	linkChildStatus := make(map[string]models.Issue)
	for i := 0; i < len(linkChildKeys); i += batchSize {
		end := i + batchSize
		if end > len(linkChildKeys) {
			end = len(linkChildKeys)
		}
		batch := linkChildKeys[i:end]
		quoted := make([]string, len(batch))
		for j, k := range batch {
			quoted[j] = fmt.Sprintf("%q", k)
		}
		childJQL := fmt.Sprintf(`key IN (%s)`, strings.Join(quoted, ","))
		startAt = 0
		for {
			result, err := jira.SearchIssues(cfg, childJQL, "key,summary,status,issuetype", startAt)
			if err != nil {
				break
			}
			for _, issue := range result.Issues {
				linkChildStatus[issue.Key] = issue
			}
			if startAt+result.MaxResults >= result.Total {
				break
			}
			startAt += result.MaxResults
		}
	}

	// Update link-based children with real statuses
	for storyKey, children := range childMap {
		for i, child := range children {
			if child.Fields.Status.Name == "" {
				if real, ok := linkChildStatus[child.Key]; ok {
					childMap[storyKey][i] = real
				}
			}
		}
	}

	// 5. Find stories with incomplete children
	doneStatuses := map[string]bool{"Готово": true, "Отклонено": true}

	type incompleteStory struct {
		storyKey     string
		storySummary string
		openChildren []models.Issue
		totalChildren int
	}
	var results []incompleteStory

	for _, story := range stories {
		children := childMap[story.Key]
		if len(children) == 0 {
			continue
		}
		// Deduplicate children by key
		seen := make(map[string]bool)
		var unique []models.Issue
		for _, c := range children {
			if !seen[c.Key] {
				seen[c.Key] = true
				unique = append(unique, c)
			}
		}

		var open []models.Issue
		for _, c := range unique {
			if !doneStatuses[c.Fields.Status.Name] {
				open = append(open, c)
			}
		}
		if len(open) > 0 {
			results = append(results, incompleteStory{
				storyKey:      story.Key,
				storySummary:  story.Fields.Summary,
				openChildren:  open,
				totalChildren: len(unique),
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return len(results[i].openChildren) > len(results[j].openChildren)
	})

	out.Printf("Историй «Готово»: %d | С незавершёнными задачами: %d", len(stories), len(results))

	if len(results) == 0 {
		out.Printf("Все дочерние задачи завершены.")
		return nil
	}

	headers := []string{"История", "Название истории", "Задача", "Тип", "Статус", "Название задачи"}
	rows := make([][]string, 0)
	for _, r := range results {
		for _, c := range r.openChildren {
			rows = append(rows, []string{
				r.storyKey,
				r.storySummary,
				c.Key,
				c.Fields.IssueType.Name,
				c.Fields.Status.Name,
				c.Fields.Summary,
			})
		}
	}
	out.SendTable(headers, rows)

	return nil
}
