package functions

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

func RunNoFixVersion(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
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

	jql := fmt.Sprintf(`%s AND fixVersion is EMPTY AND issuetype != "История" AND status NOT IN ("Ревью","В работе") AND (labels is EMPTY OR labels != "ГЛК")`, projectFilter)

	var allIssues []models.Issue
	startAt := 0
	for {
		result, err := jira.SearchIssues(cfg, jql, "key,summary,status,issuetype", startAt)
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

	ecpPattern := regexp.MustCompile(`ECP-\d+`)

	type issueWithLinks struct {
		issue     models.Issue
		linkCount int
	}

	var found []issueWithLinks
	for i, issue := range allIssues {
		out.SendProgress(i+1, len(allIssues))
		links, err := jira.FetchRemoteLinks(cfg, issue.Key)
		if err != nil {
			continue
		}

		hasECP := false
		count := 0
		for _, link := range links {
			if !strings.Contains(link.Object.URL, "gitlab.bft.local") {
				continue
			}
			if ecpPattern.MatchString(link.Object.Title) {
				hasECP = true
				break
			}
			count++
		}
		if hasECP {
			continue
		}

		if count > 0 {
			found = append(found, issueWithLinks{issue: issue, linkCount: count})
		}
	}

	// Sort by issue key numerically
	sort.Slice(found, func(i, j int) bool {
		ki := found[i].issue.Key
		kj := found[j].issue.Key
		ni, nj := 0, 0
		if idx := strings.LastIndex(ki, "-"); idx >= 0 {
			ni, _ = strconv.Atoi(ki[idx+1:])
		}
		if idx := strings.LastIndex(kj, "-"); idx >= 0 {
			nj, _ = strconv.Atoi(kj[idx+1:])
		}
		return ni < nj
	})

	out.Printf("Проекты: %s | Без fixVersion: %d | С коммитами/MR: %d",
		strings.Join(projects, ", "), len(allIssues), len(found))

	if len(found) == 0 {
		out.Printf("Не найдено задач без fixVersion с привязанными коммитами/MR.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tТИП\tСТАТУС\tКОММИТЫ/MR\tНАЗВАНИЕ")
	fmt.Fprintln(w, "---\t---\t------\t----------\t--------")
	for _, f := range found {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
			f.issue.Key, f.issue.Fields.IssueType.Name, f.issue.Fields.Status.Name,
			f.linkCount, f.issue.Fields.Summary)
	}
	w.Flush()

	return nil
}
