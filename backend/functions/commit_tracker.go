package functions

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"jira-tools-web/gitlab"
	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

// serviceName extracts the last path component as the service name.
// e.g. "ecp/skl/skl-common" → "skl-common"
func serviceName(projectPath string) string {
	if idx := strings.LastIndex(projectPath, "/"); idx >= 0 {
		return projectPath[idx+1:]
	}
	return projectPath
}

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
	if glCfg.URL == "" || glCfg.Token == "" {
		return fmt.Errorf("настройте GitLab (URL, токен) на странице конфигурации")
	}
	glc := gitlab.Config{URL: glCfg.URL, Token: glCfg.Token}

	// 1. Collect GitLab links from remote links
	out.Printf("Загрузка ссылок задачи %s...", issueKey)

	type linkKey struct{ t, p, id string }
	seenLink := map[linkKey]bool{}
	var mrLinks []gitlab.ParsedLink
	var commitLinks []gitlab.ParsedLink

	addLink := func(pl gitlab.ParsedLink) {
		var k linkKey
		if pl.Type == "mr" {
			k = linkKey{"mr", pl.ProjectPath, fmt.Sprintf("%d", pl.MRIID)}
		} else {
			k = linkKey{"commit", pl.ProjectPath, pl.CommitSHA}
		}
		if seenLink[k] {
			return
		}
		seenLink[k] = true
		switch pl.Type {
		case "mr":
			mrLinks = append(mrLinks, pl)
		case "commit":
			commitLinks = append(commitLinks, pl)
		}
	}

	remoteLinks, err := jira.FetchRemoteLinks(cfg, issueKey)
	if err != nil {
		out.Printf("  Не удалось загрузить remote links: %v", err)
	}
	for _, rl := range remoteLinks {
		if parsed := gitlab.ParseGitLabURL(glCfg.URL, rl.Object.URL); parsed != nil {
			addLink(*parsed)
		}
	}

	if len(mrLinks) == 0 && len(commitLinks) == 0 {
		out.Printf("В задаче нет ссылок на GitLab.")
		return nil
	}
	out.Printf("Найдено ссылок: %d MR, %d коммитов", len(mrLinks), len(commitLinks))

	// 2. Fetch MR details and their commits
	type mrGroup struct {
		mr       gitlab.MergeRequest
		project  string
		commits  []gitlab.Commit
		services map[string]bool // service names from commit projects
	}
	var groups []mrGroup
	seenCommit := map[string]bool{}

	for _, link := range mrLinks {
		mr, err := gitlab.GetMergeRequest(glc, link.ProjectPath, link.MRIID)
		if err != nil {
			out.Printf("  Не удалось загрузить MR !%d: %v", link.MRIID, err)
			continue
		}
		commits, err := gitlab.GetMRCommits(glc, link.ProjectPath, link.MRIID)
		if err != nil {
			out.Printf("  Не удалось загрузить коммиты MR !%d: %v", link.MRIID, err)
			continue
		}
		g := mrGroup{
			mr:       *mr,
			project:  link.ProjectPath,
			services: map[string]bool{serviceName(link.ProjectPath): true},
		}
		for _, c := range commits {
			if !seenCommit[c.ID] {
				seenCommit[c.ID] = true
				g.commits = append(g.commits, c)
			}
		}
		groups = append(groups, g)
	}

	// 3. Orphan commits (linked directly, not part of any MR)
	var orphanCommits []struct {
		commit  gitlab.Commit
		project string
	}
	for _, link := range commitLinks {
		if seenCommit[link.CommitSHA] {
			continue
		}
		c, err := gitlab.GetCommit(glc, link.ProjectPath, link.CommitSHA)
		if err != nil {
			out.Printf("  Не удалось загрузить коммит %s: %v", link.CommitSHA[:8], err)
			continue
		}
		seenCommit[c.ID] = true
		orphanCommits = append(orphanCommits, struct {
			commit  gitlab.Commit
			project string
		}{commit: *c, project: link.ProjectPath})
	}

	// Group orphan commits into a synthetic group per service
	orphanByService := map[string]*mrGroup{}
	for _, oc := range orphanCommits {
		svc := serviceName(oc.project)
		g, ok := orphanByService[svc]
		if !ok {
			g = &mrGroup{
				project:  oc.project,
				services: map[string]bool{svc: true},
			}
			orphanByService[svc] = g
		}
		g.commits = append(g.commits, oc.commit)
	}
	for _, g := range orphanByService {
		groups = append(groups, *g)
	}

	// Filter out merge commits and resolved commits
	resolved, _ := models.GetResolvedCommits(db, issueKey)
	for i := range groups {
		var filtered []gitlab.Commit
		for _, c := range groups[i].commits {
			if strings.HasPrefix(c.Title, "Merge ") {
				continue
			}
			if resolved[c.ID] {
				continue
			}
			filtered = append(filtered, c)
		}
		groups[i].commits = filtered
	}
	{
		var nonEmpty []mrGroup
		for _, g := range groups {
			if len(g.commits) > 0 {
				nonEmpty = append(nonEmpty, g)
			}
		}
		groups = nonEmpty
	}

	if len(groups) == 0 {
		out.Printf("Не найдено MR или коммитов.")
		return nil
	}

	// 4. List release branches per project
	projectSet := map[string]bool{}
	for _, g := range groups {
		projectSet[g.project] = true
	}

	var allBranchNames []string
	branchNameSet := map[string]bool{}
	projectBranches := map[string]map[string]bool{}

	for proj := range projectSet {
		branches, err := gitlab.ListBranches(glc, proj, branchPrefix)
		if err != nil {
			continue
		}
		pb := map[string]bool{}
		for _, b := range branches {
			pb[b.Name] = true
			if !branchNameSet[b.Name] {
				branchNameSet[b.Name] = true
				allBranchNames = append(allBranchNames, b.Name)
			}
		}
		projectBranches[proj] = pb
	}
	sort.Slice(allBranchNames, func(i, j int) bool {
		return branchNum(allBranchNames[i]) < branchNum(allBranchNames[j])
	})
	// Keep only last 3 release branches
	if len(allBranchNames) > 3 {
		allBranchNames = allBranchNames[len(allBranchNames)-3:]
	}
	out.Printf("Релизных веток: %d (показаны последние)", len(allBranchNames))

	// 6. For each group, check each commit in each release branch
	type branchStatus struct {
		total   int
		found   int
		missing []gitlab.Commit
	}
	type groupResult struct {
		group    mrGroup
		branches map[string]branchStatus
	}
	var results []groupResult

	totalWork := 0
	for _, g := range groups {
		totalWork += len(g.commits) * len(allBranchNames)
	}
	progress := 0

	for _, g := range groups {
		gr := groupResult{
			group:    g,
			branches: make(map[string]branchStatus),
		}

		pb := projectBranches[g.project]
		for _, bname := range allBranchNames {
			bs := branchStatus{total: len(g.commits)}

			if !pb[bname] {
				progress += len(g.commits)
				if totalWork > 0 {
					out.SendProgress(progress, totalWork)
				}
				gr.branches[bname] = bs
				continue
			}

			// Search commits with the issue key on this branch
			candidates, _ := gitlab.SearchCommits(glc, g.project, bname, issueKey)

			for _, orig := range g.commits {
				progress++
				if totalWork > 0 {
					out.SendProgress(progress, totalWork)
				}

				matched := false
				for _, cand := range candidates {
					// Exact SHA match
					if cand.ID == orig.ID {
						matched = true
						break
					}
					// Title match (cherry-picked commits keep the same message)
					if cand.Title == orig.Title {
						matched = true
						break
					}
				}
				if matched {
					bs.found++
				} else {
					bs.missing = append(bs.missing, orig)
				}
			}
			gr.branches[bname] = bs
		}
		results = append(results, gr)
	}

	// 7. Build aggregated table
	headers := []string{"MR", "Сервис", "Коммитов"}
	headers = append(headers, allBranchNames...)

	glBase := gitlab.BaseHost(glCfg.URL)
	rows := make([][]string, 0, len(results))
	for _, gr := range results {
		mrCell := "—"
		if gr.group.mr.IID > 0 {
			title := fmt.Sprintf("!%d %s", gr.group.mr.IID, truncate(gr.group.mr.Title, 50))
			link := fmt.Sprintf("%s/%s/-/merge_requests/%d", glBase, gr.group.project, gr.group.mr.IID)
			mrCell = fmt.Sprintf("[%s](%s)", title, link)
		}

		svcs := make([]string, 0, len(gr.group.services))
		for s := range gr.group.services {
			svcs = append(svcs, s)
		}
		sort.Strings(svcs)

		row := []string{
			mrCell,
			strings.Join(svcs, ", "),
			fmt.Sprintf("%d", len(gr.group.commits)),
		}

		for _, bname := range allBranchNames {
			bs := gr.branches[bname]
			if bs.found == 0 {
				row = append(row, "—")
			} else if bs.found == bs.total {
				row = append(row, "✓")
			} else {
				cell := fmt.Sprintf("%d/%d", bs.found, bs.total)
				if len(bs.missing) > 0 {
					var parts []string
					for _, m := range bs.missing {
						parts = append(parts, m.ID+"\t"+m.Title)
					}
					cell += "||" + issueKey + "||" + strings.Join(parts, "\n")
				}
				row = append(row, cell)
			}
		}
		rows = append(rows, row)
	}
	out.SendTable(headers, rows)

	return nil
}

// branchNum extracts trailing number from branch name for numeric sorting.
// e.g. "release-99" → 99, "release-100" → 100
func branchNum(name string) int {
	if idx := strings.LastIndex(name, "-"); idx >= 0 {
		if n, err := strconv.Atoi(name[idx+1:]); err == nil {
			return n
		}
	}
	return 0
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
