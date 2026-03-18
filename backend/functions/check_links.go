package functions

import (
	"fmt"
	"strings"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

func RunCheckLinks(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	projects := strings.Split(params["project"], ",")
	for i := range projects {
		projects[i] = strings.TrimSpace(projects[i])
	}
	fixParentof := params["fix_parentof"] == "true"

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

	jql := fmt.Sprintf(`%s AND issuetype = "Задача"`, projectFilter)

	var allIssues []models.Issue
	startAt := 0
	for {
		result, err := jira.SearchIssuesDefault(cfg, jql, startAt)
		if err != nil {
			return fmt.Errorf("ошибка: %v", err)
		}
		allIssues = append(allIssues, result.Issues...)
		out.SendProgress(len(allIssues), result.Total)
		if startAt+result.MaxResults >= result.Total {
			break
		}
		startAt += result.MaxResults
	}

	type mismatch struct {
		taskKey  string
		taskName string
		author   string
		storyKey string
		linkID   string
		linkType string
		linkDesc string
	}

	var mismatches []mismatch
	linkedCount := 0

	for _, issue := range allIssues {
		for _, link := range issue.Fields.IssueLinks {
			var storyKey, desc string
			isStory := false
			storyOnInward := false

			if link.InwardIssue != nil && link.InwardIssue.Fields.IssueType.Name == "История" {
				isStory = true
				storyKey = link.InwardIssue.Key
				desc = link.Type.Inward
				storyOnInward = true
			}
			if link.OutwardIssue != nil && link.OutwardIssue.Fields.IssueType.Name == "История" {
				isStory = true
				storyKey = link.OutwardIssue.Key
				desc = link.Type.Outward
			}

			if !isStory {
				continue
			}
			linkedCount++

			lower := strings.ToLower(link.Type.Name + " " + link.Type.Inward + " " + link.Type.Outward)
			isParentType := strings.Contains(lower, "parent")

			wrongType := !isParentType
			wrongDirection := isParentType && storyOnInward

			if wrongType || wrongDirection {
				reason := desc
				if wrongDirection {
					reason = desc + " (неверное направление)"
				}
				mismatches = append(mismatches, mismatch{
					taskKey:  issue.Key,
					taskName: issue.Fields.Summary,
					author:   jira.FormatAuthor(issue),
					storyKey: storyKey,
					linkID:   link.ID,
					linkType: link.Type.Name,
					linkDesc: reason,
				})
			}
		}
	}

	out.Printf("Проекты: %s | Задач: %d | Со связью к истории: %d | Несоответствий: %d",
		strings.Join(projects, ", "), len(allIssues), linkedCount, len(mismatches))

	if len(mismatches) == 0 {
		out.Printf("Все связи к историям корректны (parentof).")
		return nil
	}

	headers := []string{"Автор", "Задача", "История", "Тип связи", "Описание", "Название задачи"}
	rows := make([][]string, 0, len(mismatches))
	for _, m := range mismatches {
		rows = append(rows, []string{m.author, m.taskKey, m.storyKey, m.linkType, m.linkDesc, m.taskName})
	}
	out.SendTable(headers, rows)

	// Fix links
	if fixParentof {
		parentType, err := jira.GetParentLinkType(cfg)
		if err != nil {
			return fmt.Errorf("ошибка: %v", err)
		}
		out.Printf("Найден тип связи: %s (inward: %q, outward: %q)", parentType.Name, parentType.Inward, parentType.Outward)

		storyIsOutward := strings.Contains(strings.ToLower(parentType.Inward), "parent")

		var toFix []mismatch
		for _, m := range mismatches {
			lower := strings.ToLower(m.linkType)
			if strings.Contains(lower, "relat") || strings.Contains(lower, "parent") {
				toFix = append(toFix, m)
			}
		}

		if len(toFix) == 0 {
			out.Printf("Нет связей для исправления.")
			return nil
		}

		out.Printf("Исправление %d связей → %s...", len(toFix), parentType.Name)
		fixed := 0
		errors := 0
		for _, m := range toFix {
			if err := jira.DeleteIssueLink(cfg, m.linkID); err != nil {
				out.Printf("[ОШИБКА] удаление связи у %s: %v", m.taskKey, err)
				errors++
				continue
			}
			var inward, outward string
			if storyIsOutward {
				inward = m.taskKey
				outward = m.storyKey
			} else {
				inward = m.storyKey
				outward = m.taskKey
			}
			if err := jira.CreateIssueLink(cfg, parentType.Name, inward, outward); err != nil {
				out.Printf("[ОШИБКА] создание связи %s → %s: %v", m.storyKey, m.taskKey, err)
				errors++
			} else {
				fixed++
				out.SendProgress(fixed, len(toFix))
			}
		}
		out.Printf("Готово: %d исправлено, %d ошибок", fixed, errors)
	}

	return nil
}
