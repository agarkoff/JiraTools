package functions

import (
	"fmt"
	"regexp"
	"strings"

	"jira-tools-web/gitlab"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

func RunCommitTracker(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	issueKey := strings.TrimSpace(params["issue_key"])
	if issueKey == "" {
		return fmt.Errorf("не указан ключ задачи")
	}
	branchPrefix := strings.TrimSpace(params["branch_prefix"])
	if branchPrefix == "" {
		branchPrefix = "release-"
	}

	db := out.DB()
	glCfg, err := models.LoadGitLabConfig(db)
	if err != nil {
		return fmt.Errorf("ошибка загрузки конфига GitLab: %v", err)
	}
	if glCfg.URL == "" || glCfg.Token == "" || glCfg.Project == "" {
		return fmt.Errorf("настройте GitLab (URL, токен, проект) на странице конфигурации")
	}
	glc := gitlab.Config{URL: glCfg.URL, Token: glCfg.Token, Project: glCfg.Project}

	// 1. Find MRs
	out.Printf("Поиск MR по ключу %s...", issueKey)
	mrs, err := gitlab.SearchMergeRequests(glc, issueKey)
	if err != nil {
		return fmt.Errorf("ошибка поиска MR: %v", err)
	}

	// Filter MRs that actually contain the issue key
	keyRe := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(issueKey) + `\b`)
	var matchedMRs []gitlab.MergeRequest
	for _, mr := range mrs {
		if keyRe.MatchString(mr.Title) || keyRe.MatchString(mr.SourceBranch) {
			matchedMRs = append(matchedMRs, mr)
		}
	}

	if len(matchedMRs) > 0 {
		mrHeaders := []string{"MR", "Название", "Автор", "Из ветки", "В ветку", "Статус", "Ссылка"}
		mrRows := make([][]string, 0, len(matchedMRs))
		for _, mr := range matchedMRs {
			mrRows = append(mrRows, []string{
				fmt.Sprintf("!%d", mr.IID),
				mr.Title,
				mr.Author.Name,
				mr.SourceBranch,
				mr.TargetBranch,
				mr.State,
				mr.WebURL,
			})
		}
		out.SendTable(mrHeaders, mrRows)
	} else {
		out.Printf("MR не найдены.")
	}

	// 2. Find commits on default branch
	out.Printf("Поиск коммитов в основной ветке...")
	// Try common default branch names
	var mainCommits []gitlab.Commit
	for _, branch := range []string{"master", "main", "develop"} {
		commits, err := gitlab.SearchCommits(glc, branch, issueKey)
		if err != nil {
			continue
		}
		for _, c := range commits {
			if keyRe.MatchString(c.Message) {
				mainCommits = append(mainCommits, c)
			}
		}
		if len(mainCommits) > 0 {
			out.Printf("Найдено %d коммитов в ветке %s", len(mainCommits), branch)
			break
		}
	}

	// Also collect commits from MR source branches
	for _, mr := range matchedMRs {
		commits, err := gitlab.SearchCommits(glc, mr.SourceBranch, issueKey)
		if err != nil {
			continue
		}
		seen := make(map[string]bool)
		for _, c := range mainCommits {
			seen[c.ID] = true
		}
		for _, c := range commits {
			if !seen[c.ID] && keyRe.MatchString(c.Message) {
				mainCommits = append(mainCommits, c)
			}
		}
	}

	if len(mainCommits) == 0 {
		out.Printf("Коммиты с ключом %s не найдены.", issueKey)
		return nil
	}

	// 3. Get diffs for main commits
	out.Printf("Загрузка diff для %d коммитов...", len(mainCommits))
	type commitWithDiff struct {
		commit gitlab.Commit
		diff   []string // normalized
	}
	var originals []commitWithDiff
	for _, c := range mainCommits {
		diffs, err := gitlab.GetCommitDiff(glc, c.ID)
		if err != nil {
			out.Printf("  Не удалось загрузить diff %s: %v", c.ShortID, err)
			continue
		}
		originals = append(originals, commitWithDiff{
			commit: c,
			diff:   gitlab.NormalizeDiff(diffs),
		})
	}

	// 4. List release branches
	out.Printf("Поиск релизных веток (%s*)...", branchPrefix)
	branches, err := gitlab.ListBranches(glc, branchPrefix)
	if err != nil {
		return fmt.Errorf("ошибка загрузки веток: %v", err)
	}
	out.Printf("Найдено %d релизных веток", len(branches))

	// 5. For each original commit, check presence in each release branch
	// Result: commit -> branch -> match info
	type branchMatch struct {
		found      bool
		matchSHA   string
		similarity float64
	}
	type commitResult struct {
		original commitWithDiff
		branches map[string]branchMatch
	}
	var results []commitResult

	total := len(originals) * len(branches)
	progress := 0

	for _, orig := range originals {
		cr := commitResult{
			original: orig,
			branches: make(map[string]branchMatch),
		}

		for _, branch := range branches {
			progress++
			out.SendProgress(progress, total)

			// Search by commit message (first line) in this branch
			searchTerm := firstLine(orig.commit.Title)
			if len(searchTerm) > 100 {
				searchTerm = searchTerm[:100]
			}

			candidates, err := gitlab.SearchCommits(glc, branch.Name, issueKey)
			if err != nil {
				cr.branches[branch.Name] = branchMatch{found: false}
				continue
			}

			// Check each candidate
			bestMatch := branchMatch{found: false}
			for _, cand := range candidates {
				if !keyRe.MatchString(cand.Message) {
					continue
				}

				// Exact SHA match (same commit, e.g. merged into both)
				if cand.ID == orig.commit.ID {
					bestMatch = branchMatch{found: true, matchSHA: cand.ShortID, similarity: 1.0}
					break
				}

				// Compare diffs
				candDiffs, err := gitlab.GetCommitDiff(glc, cand.ID)
				if err != nil {
					continue
				}
				candNorm := gitlab.NormalizeDiff(candDiffs)
				sim := gitlab.DiffSimilarity(orig.diff, candNorm)
				if sim > bestMatch.similarity {
					bestMatch = branchMatch{found: sim >= 0.7, matchSHA: cand.ShortID, similarity: sim}
				}
			}
			cr.branches[branch.Name] = bestMatch
		}
		results = append(results, cr)
	}

	// 6. Build result table
	// Headers: Коммит | Сообщение | Автор | branch1 | branch2 | ...
	branchNames := make([]string, len(branches))
	for i, b := range branches {
		branchNames[i] = b.Name
	}

	headers := []string{"Коммит", "Сообщение", "Автор"}
	headers = append(headers, branchNames...)

	rows := make([][]string, 0, len(results))
	for _, cr := range results {
		row := []string{
			cr.original.commit.ShortID,
			truncate(cr.original.commit.Title, 60),
			cr.original.commit.AuthorName,
		}
		for _, bname := range branchNames {
			m := cr.branches[bname]
			if !m.found {
				row = append(row, "—")
			} else if m.similarity >= 0.99 {
				row = append(row, fmt.Sprintf("%s", m.matchSHA))
			} else {
				row = append(row, fmt.Sprintf("%s (%.0f%%)", m.matchSHA, m.similarity*100))
			}
		}
		rows = append(rows, row)
	}
	out.SendTable(headers, rows)

	return nil
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
