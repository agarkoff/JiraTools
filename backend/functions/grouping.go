package functions

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"jira-tools-web/jira"
	"jira-tools-web/llm"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

// --- TF-IDF + cosine similarity for task grouping ---

var ruStopWords = map[string]bool{
	"и": true, "в": true, "на": true, "не": true, "по": true, "для": true, "из": true,
	"от": true, "за": true, "что": true, "как": true, "это": true, "при": true, "до": true,
	"без": true, "под": true, "над": true, "между": true, "через": true, "после": true,
	"перед": true, "или": true, "но": true, "да": true, "нет": true, "бы": true,
	"же": true, "ли": true, "все": true, "его": true, "так": true, "уже": true,
	"вы": true, "мы": true, "он": true, "она": true, "они": true, "ее": true, "их": true,
	"быть": true, "был": true, "была": true, "были": true, "будет": true,
	"также": true, "если": true, "того": true, "этого": true, "еще": true,
	"нужно": true, "надо": true, "можно": true, "чтобы": true, "когда": true,
	"есть": true, "нужна": true, "необходимо": true, "должен": true, "может": true,
	"только": true, "более": true, "очень": true, "этот": true, "эта": true,
	// Common Jira noise words
	"задача": true, "ошибка": true, "баг": true, "bug": true, "task": true,
	"сделать": true, "добавить": true, "исправить": true, "реализовать": true,
	"fix": true, "add": true, "implement": true, "update": true, "create": true,
}

func tokenize(text string) []string {
	lower := strings.ToLower(text)
	var tokens []string
	var current []rune
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
		} else {
			if len(current) >= 3 {
				word := string(current)
				if !ruStopWords[word] {
					tokens = append(tokens, word)
				}
			}
			current = current[:0]
		}
	}
	if len(current) >= 3 {
		word := string(current)
		if !ruStopWords[word] {
			tokens = append(tokens, word)
		}
	}
	return tokens
}

type tfidfModel struct {
	idf  map[string]float64
	docs int
}

func buildTFIDF(documents [][]string) *tfidfModel {
	df := make(map[string]int)
	for _, doc := range documents {
		seen := make(map[string]bool)
		for _, word := range doc {
			if !seen[word] {
				df[word]++
				seen[word] = true
			}
		}
	}
	idf := make(map[string]float64, len(df))
	n := float64(len(documents))
	for word, count := range df {
		idf[word] = math.Log(1 + n/float64(count))
	}
	return &tfidfModel{idf: idf, docs: len(documents)}
}

func (m *tfidfModel) vectorize(tokens []string) map[string]float64 {
	tf := make(map[string]int)
	for _, t := range tokens {
		tf[t]++
	}
	vec := make(map[string]float64, len(tf))
	for word, count := range tf {
		if idf, ok := m.idf[word]; ok {
			vec[word] = float64(count) * idf
		}
	}
	return vec
}

func cosineSim(a, b map[string]float64) float64 {
	var dot, normA, normB float64
	for k, v := range a {
		normA += v * v
		if bv, ok := b[k]; ok {
			dot += v * bv
		}
	}
	for _, v := range b {
		normB += v * v
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

type issueGroup struct {
	issues     []models.Issue
	storyKey   string
	storyTitle string
	score      float64
}

func RunGroupOrphans(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	projects := strings.Split(params["project"], ",")
	for i := range projects {
		projects[i] = strings.TrimSpace(projects[i])
	}

	const clusterThreshold = 0.4
	const storyThreshold = 0.2

	// --- 1. Load orphan tasks and bugs ---
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

	jql := fmt.Sprintf(`%s AND issuetype IN ("Задача","Ошибка")`, projectFilter)
	fields := "key,summary,description,status,creator,parent,issuelinks,issuetype"
	var allIssues []models.Issue
	startAt := 0
	for {
		result, err := jira.SearchIssues(cfg, jql, fields, startAt)
		if err != nil {
			return fmt.Errorf("ошибка загрузки задач: %v", err)
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

	out.Printf("Всего задач/ошибок: %d | Без истории: %d", len(allIssues), len(orphans))

	if len(orphans) == 0 {
		out.Printf("Все задачи привязаны к историям.")
		return nil
	}

	// --- 2. Load stories ---
	storyJQL := fmt.Sprintf(`%s AND issuetype = "История"`, projectFilter)
	var stories []models.Issue
	startAt = 0
	for {
		result, err := jira.SearchIssues(cfg, storyJQL, "key,summary,description,status", startAt)
		if err != nil {
			return fmt.Errorf("ошибка загрузки историй: %v", err)
		}
		stories = append(stories, result.Issues...)
		if startAt+result.MaxResults >= result.Total {
			break
		}
		startAt += result.MaxResults
	}
	out.Printf("Историй для сопоставления: %d", len(stories))

	// --- 3. Choose mode ---
	useLLM := params["mode"] == "llm"
	var llmCfg llm.Config
	if useLLM {
		lc, err := models.LoadLLMConfig(out.DB())
		if err != nil || lc.URL == "" || lc.Model == "" {
			return fmt.Errorf("Ollama не настроена. Укажите URL и модель в Настройках")
		}
		llmCfg = llm.Config{URL: lc.URL, Model: lc.Model}
		out.Printf("Режим: LLM (%s)", llmCfg.Model)
	} else {
		out.Printf("Режим: TF-IDF")
	}

	var realGroups []issueGroup
	var singletons []models.Issue

	if useLLM {
		realGroups, singletons = groupWithLLM(orphans, stories, llmCfg, out)
	} else {
		realGroups, singletons = groupWithTFIDF(orphans, stories)
	}

	// --- Output ---
	out.Printf("Групп (2+ задачи): %d | Без группы: %d", len(realGroups), len(singletons))

	headers := []string{"Key", "Тип", "Статус", "Автор", "Название"}

	for gi, g := range realGroups {
		groupName := fmt.Sprintf("Группа %d (%d задач)", gi+1, len(g.issues))
		if g.storyKey != "" {
			groupName += fmt.Sprintf(" → %s: %s", g.storyKey, g.storyTitle)
		}

		rows := make([][]string, 0, len(g.issues))
		for _, issue := range g.issues {
			rows = append(rows, []string{
				issue.Key,
				issue.Fields.IssueType.Name,
				issue.Fields.Status.Name,
				jira.FormatAuthor(issue),
				issue.Fields.Summary,
			})
		}
		out.SendGroupedTable(groupName, "group", headers, rows)
	}

	if len(singletons) > 0 {
		rows := make([][]string, 0, len(singletons))
		for _, issue := range singletons {
			rows = append(rows, []string{
				issue.Key,
				issue.Fields.IssueType.Name,
				issue.Fields.Status.Name,
				jira.FormatAuthor(issue),
				issue.Fields.Summary,
			})
		}
		out.SendTable(headers, rows)
	}

	return nil
}

// --- TF-IDF grouping ---

func groupWithTFIDF(orphans, stories []models.Issue) ([]issueGroup, []models.Issue) {
	const clusterThreshold = 0.4
	const storyThreshold = 0.2

	issueText := func(issue models.Issue) string {
		text := issue.Fields.Summary
		if issue.Fields.Description != "" {
			text += " " + issue.Fields.Description
		}
		return text
	}

	orphanTokens := make([][]string, len(orphans))
	for i, o := range orphans {
		orphanTokens[i] = tokenize(issueText(o))
	}
	storyTokens := make([][]string, len(stories))
	for i, s := range stories {
		storyTokens[i] = tokenize(issueText(s))
	}

	allDocs := make([][]string, 0, len(orphanTokens)+len(storyTokens))
	allDocs = append(allDocs, orphanTokens...)
	allDocs = append(allDocs, storyTokens...)
	model := buildTFIDF(allDocs)

	orphanVecs := make([]map[string]float64, len(orphans))
	for i, tokens := range orphanTokens {
		orphanVecs[i] = model.vectorize(tokens)
	}
	storyVecs := make([]map[string]float64, len(stories))
	for i, tokens := range storyTokens {
		storyVecs[i] = model.vectorize(tokens)
	}

	orphanIdx := make(map[string]int, len(orphans))
	for i, o := range orphans {
		orphanIdx[o.Key] = i
	}

	computeCentroid := func(indices []int) map[string]float64 {
		centroid := make(map[string]float64)
		for _, idx := range indices {
			for k, v := range orphanVecs[idx] {
				centroid[k] += v
			}
		}
		n := float64(len(indices))
		for k := range centroid {
			centroid[k] /= n
		}
		return centroid
	}

	assigned := make([]bool, len(orphans))
	var groups []issueGroup

	for {
		seed := -1
		for i, a := range assigned {
			if !a {
				seed = i
				break
			}
		}
		if seed == -1 {
			break
		}

		memberIdxs := []int{seed}
		assigned[seed] = true

		changed := true
		for changed {
			changed = false
			centroid := computeCentroid(memberIdxs)
			for i := range orphans {
				if assigned[i] {
					continue
				}
				if cosineSim(centroid, orphanVecs[i]) >= clusterThreshold {
					memberIdxs = append(memberIdxs, i)
					assigned[i] = true
					changed = true
				}
			}
		}

		issues := make([]models.Issue, len(memberIdxs))
		for i, idx := range memberIdxs {
			issues[i] = orphans[idx]
		}
		groups = append(groups, issueGroup{issues: issues})
	}

	var realGroups []issueGroup
	var singletons []models.Issue
	for _, g := range groups {
		if len(g.issues) >= 2 {
			realGroups = append(realGroups, g)
		} else {
			singletons = append(singletons, g.issues[0])
		}
	}

	sort.Slice(realGroups, func(i, j int) bool {
		return len(realGroups[i].issues) > len(realGroups[j].issues)
	})

	for gi := range realGroups {
		var memberIdxs []int
		for _, issue := range realGroups[gi].issues {
			if idx, ok := orphanIdx[issue.Key]; ok {
				memberIdxs = append(memberIdxs, idx)
			}
		}
		centroid := computeCentroid(memberIdxs)

		bestScore := 0.0
		bestIdx := -1
		for si := range stories {
			sim := cosineSim(centroid, storyVecs[si])
			if sim > bestScore {
				bestScore = sim
				bestIdx = si
			}
		}
		if bestIdx >= 0 && bestScore >= storyThreshold {
			realGroups[gi].storyKey = stories[bestIdx].Key
			realGroups[gi].storyTitle = stories[bestIdx].Fields.Summary
			realGroups[gi].score = bestScore
		}
	}

	return realGroups, singletons
}

// --- LLM grouping ---

type llmGroupResult struct {
	Groups []struct {
		StoryKey string   `json:"story_key"`
		TaskKeys []string `json:"task_keys"`
	} `json:"groups"`
	Ungrouped []string `json:"ungrouped"`
}

func groupWithLLM(orphans, stories []models.Issue, cfg llm.Config, out *sse.Writer) ([]issueGroup, []models.Issue) {
	// Build compact lists for the prompt
	var taskLines []string
	for _, o := range orphans {
		taskLines = append(taskLines, fmt.Sprintf("%s: %s", o.Key, o.Fields.Summary))
	}
	var storyLines []string
	for _, s := range stories {
		storyLines = append(storyLines, fmt.Sprintf("%s: %s", s.Key, s.Fields.Summary))
	}

	// Send in batches if too many orphans
	batchSize := 80
	if len(orphans) <= batchSize {
		return llmGroupBatch(taskLines, storyLines, orphans, stories, cfg, out)
	}

	// Process in batches
	var allGroups []issueGroup
	var allSingletons []models.Issue
	for i := 0; i < len(orphans); i += batchSize {
		end := i + batchSize
		if end > len(orphans) {
			end = len(orphans)
		}
		batchOrphans := orphans[i:end]
		batchLines := taskLines[i:end]
		out.Printf("Обработка пакета %d-%d из %d...", i+1, end, len(orphans))
		groups, singles := llmGroupBatch(batchLines, storyLines, batchOrphans, stories, cfg, out)
		allGroups = append(allGroups, groups...)
		allSingletons = append(allSingletons, singles...)
	}

	sort.Slice(allGroups, func(i, j int) bool {
		return len(allGroups[i].issues) > len(allGroups[j].issues)
	})
	return allGroups, allSingletons
}

func llmGroupBatch(taskLines, storyLines []string, orphans, stories []models.Issue, cfg llm.Config, out *sse.Writer) ([]issueGroup, []models.Issue) {
	prompt := fmt.Sprintf(`/no_think
You are a Jira task analyzer. Group orphan tasks by semantic similarity and match each group to the most relevant story.

ORPHAN TASKS (no linked story):
%s

EXISTING STORIES:
%s

Return ONLY valid JSON (no markdown, no explanation):
{"groups":[{"story_key":"STORY-KEY","task_keys":["TASK-1","TASK-2"]}],"ungrouped":["TASK-X"]}

Rules:
- Group tasks that are semantically related (same feature, module, or topic)
- Each group must have 2+ tasks
- Match each group to the single best story, or use "" if no story fits
- Tasks that don't fit any group go into "ungrouped"
- Use exact keys from the lists above`,
		strings.Join(taskLines, "\n"), strings.Join(storyLines, "\n"))

	out.Printf("Отправка запроса в LLM...")
	response, err := llm.Generate(cfg, prompt)
	if err != nil {
		out.Printf("[ОШИБКА LLM] %v — fallback на TF-IDF", err)
		return groupWithTFIDF(orphans, stories)
	}

	// Extract JSON from response (might have markdown fences)
	jsonStr := response
	if idx := strings.Index(jsonStr, "{"); idx >= 0 {
		jsonStr = jsonStr[idx:]
	}
	if idx := strings.LastIndex(jsonStr, "}"); idx >= 0 {
		jsonStr = jsonStr[:idx+1]
	}

	var result llmGroupResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		out.Printf("[ОШИБКА] Не удалось разобрать ответ LLM: %v — fallback на TF-IDF", err)
		return groupWithTFIDF(orphans, stories)
	}

	// Build index
	orphanMap := make(map[string]models.Issue, len(orphans))
	for _, o := range orphans {
		orphanMap[o.Key] = o
	}
	storyMap := make(map[string]models.Issue, len(stories))
	for _, s := range stories {
		storyMap[s.Key] = s
	}

	assigned := make(map[string]bool)
	var realGroups []issueGroup

	for _, g := range result.Groups {
		var issues []models.Issue
		for _, key := range g.TaskKeys {
			if issue, ok := orphanMap[key]; ok && !assigned[key] {
				issues = append(issues, issue)
				assigned[key] = true
			}
		}
		if len(issues) < 2 {
			continue
		}
		group := issueGroup{issues: issues}
		if story, ok := storyMap[g.StoryKey]; ok {
			group.storyKey = story.Key
			group.storyTitle = story.Fields.Summary
		}
		realGroups = append(realGroups, group)
	}

	var singletons []models.Issue
	for _, o := range orphans {
		if !assigned[o.Key] {
			singletons = append(singletons, o)
		}
	}

	sort.Slice(realGroups, func(i, j int) bool {
		return len(realGroups[i].issues) > len(realGroups[j].issues)
	})

	return realGroups, singletons
}
