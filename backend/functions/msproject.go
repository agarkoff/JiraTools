package functions

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
	"time"

	"jira-tools-web/jira"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

// --- MS Project XML structures ---

type msProjectXML struct {
	XMLName           xml.Name    `xml:"Project"`
	Xmlns             string      `xml:"xmlns,attr"`
	SaveVersion       int         `xml:"SaveVersion"`
	Name              string      `xml:"Name"`
	ScheduleFromStart int         `xml:"ScheduleFromStart"`
	StartDate         string      `xml:"StartDate"`
	FinishDate        string      `xml:"FinishDate"`
	CalendarUID       int         `xml:"CalendarUID"`
	DefaultStartTime  string      `xml:"DefaultStartTime"`
	DefaultFinishTime string      `xml:"DefaultFinishTime"`
	MinutesPerDay     int         `xml:"MinutesPerDay"`
	MinutesPerWeek    int         `xml:"MinutesPerWeek"`
	DaysPerMonth      int         `xml:"DaysPerMonth"`
	HonorConstraints  int         `xml:"HonorConstraints"`
	Calendars         msCalendars `xml:"Calendars"`
	Tasks             msTasks     `xml:"Tasks"`
}

type msCalendars struct {
	Calendar []msCalendar `xml:"Calendar"`
}

type msCalendar struct {
	UID            int        `xml:"UID"`
	Name           string     `xml:"Name"`
	IsBaseCalendar int        `xml:"IsBaseCalendar"`
	WeekDays       msWeekDays `xml:"WeekDays"`
}

type msWeekDays struct {
	WeekDay []msWeekDay `xml:"WeekDay"`
}

type msWeekDay struct {
	DayType      int             `xml:"DayType"`
	DayWorking   int             `xml:"DayWorking"`
	WorkingTimes *msWorkingTimes `xml:"WorkingTimes,omitempty"`
}

type msWorkingTimes struct {
	WorkingTime []msWorkingTime `xml:"WorkingTime"`
}

type msWorkingTime struct {
	FromTime string `xml:"FromTime"`
	ToTime   string `xml:"ToTime"`
}

type msTasks struct {
	Task []msTask `xml:"Task"`
}

type msTask struct {
	UID            int    `xml:"UID"`
	ID             int    `xml:"ID"`
	Name           string `xml:"Name"`
	Manual         int    `xml:"Manual"`
	OutlineLevel   int    `xml:"OutlineLevel"`
	Start          string `xml:"Start,omitempty"`
	Finish         string `xml:"Finish,omitempty"`
	Duration       string `xml:"Duration,omitempty"`
	DurationFormat int    `xml:"DurationFormat,omitempty"`
	ConstraintType int    `xml:"ConstraintType,omitempty"`
	ConstraintDate string `xml:"ConstraintDate,omitempty"`
	Summary        int    `xml:"Summary"`
	Milestone      int    `xml:"Milestone"`
	Notes          string `xml:"Notes,omitempty"`
}

type msStoryNode struct {
	issue   models.Issue
	epicKey string
	tasks   []models.Issue
}

func RunMSProject(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error {
	epicField := params["epic_field"]
	if epicField == "" {
		epicField = "customfield_10109"
	}

	projects := strings.Split(params["project"], ",")
	for i := range projects {
		projects[i] = strings.TrimSpace(projects[i])
	}

	for _, project := range projects {
		if err := exportMSProject(cfg, project, epicField, out); err != nil {
			return err
		}
	}
	return nil
}

func exportMSProject(cfg models.JiraConfig, project, epicField string, out *sse.Writer) error {
	out.Printf("Загрузка данных проекта %s...", project)

	// 1. Load all epics
	epicJQL := fmt.Sprintf(`project = "%s" AND issuetype = "Epic"`, project)
	var epics []models.Issue
	startAt := 0
	for {
		result, err := jira.SearchIssues(cfg, epicJQL, "key,summary,status,duedate,timetracking", startAt)
		if err != nil {
			return fmt.Errorf("ошибка загрузки эпиков: %w", err)
		}
		epics = append(epics, result.Issues...)
		if startAt+result.MaxResults >= result.Total {
			break
		}
		startAt += result.MaxResults
	}
	out.Printf("  Эпиков: %d", len(epics))

	// Index epics by key
	epicMap := make(map[string]*models.Issue)
	for i := range epics {
		epicMap[epics[i].Key] = &epics[i]
	}

	// 2. Load all stories (via raw JSON to access Epic Link custom field)
	storyJQL := fmt.Sprintf(`project = "%s" AND issuetype = "История"`, project)
	storyFields := "key,summary,status,parent,duedate,timetracking,issuelinks," + epicField
	var storyNodes []msStoryNode
	startAt = 0
	for {
		body, err := jira.DoSearch(cfg, storyJQL, storyFields, startAt)
		if err != nil {
			return fmt.Errorf("ошибка загрузки историй: %w", err)
		}
		var result models.RawSearchResult
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("ошибка парсинга историй: %w", err)
		}

		for _, raw := range result.Issues {
			sn := msStoryNode{}
			sn.issue.Key = raw.Key
			if v, ok := raw.Fields["summary"]; ok {
				json.Unmarshal(v, &sn.issue.Fields.Summary)
			}
			if v, ok := raw.Fields["status"]; ok {
				json.Unmarshal(v, &sn.issue.Fields.Status)
			}
			if v, ok := raw.Fields["duedate"]; ok {
				json.Unmarshal(v, &sn.issue.Fields.DueDate)
			}
			if v, ok := raw.Fields["timetracking"]; ok {
				var tt models.TimeTracking
				json.Unmarshal(v, &tt)
				sn.issue.Fields.TimeTracking = &tt
			}
			if v, ok := raw.Fields["issuelinks"]; ok {
				json.Unmarshal(v, &sn.issue.Fields.IssueLinks)
			}

			// Determine epic: first parent, then Epic Link
			if v, ok := raw.Fields["parent"]; ok && string(v) != "null" {
				var parent models.ParentRef
				json.Unmarshal(v, &parent)
				sn.issue.Fields.Parent = &parent
				if _, ok := epicMap[parent.Key]; ok {
					sn.epicKey = parent.Key
				}
			}
			if sn.epicKey == "" {
				if v, ok := raw.Fields[epicField]; ok && string(v) != "null" && len(v) > 0 {
					var ek string
					if err := json.Unmarshal(v, &ek); err == nil && ek != "" {
						sn.epicKey = ek
					}
				}
			}

			storyNodes = append(storyNodes, sn)
		}

		out.SendProgress(len(storyNodes), result.Total)
		if result.StartAt+result.MaxResults >= result.Total {
			break
		}
		startAt += result.MaxResults
	}
	out.Printf("  Историй: %d", len(storyNodes))

	// 3. Load all tasks
	taskJQL := fmt.Sprintf(`project = "%s" AND issuetype IN ("Задача", "Ошибка")`, project)
	var tasks []models.Issue
	startAt = 0
	for {
		result, err := jira.SearchIssues(cfg, taskJQL, "key,summary,status,parent,duedate,timetracking,issuelinks", startAt)
		if err != nil {
			return fmt.Errorf("ошибка загрузки задач: %w", err)
		}
		tasks = append(tasks, result.Issues...)
		if startAt+result.MaxResults >= result.Total {
			break
		}
		startAt += result.MaxResults
	}
	out.Printf("  Задач: %d", len(tasks))

	// 4. Build hierarchy: Epic -> Story -> Task
	storyMap := make(map[string]*msStoryNode)
	storiesByEpic := make(map[string][]msStoryNode)
	var orphanStories []msStoryNode

	for _, sn := range storyNodes {
		if sn.epicKey != "" {
			storiesByEpic[sn.epicKey] = append(storiesByEpic[sn.epicKey], sn)
		} else {
			orphanStories = append(orphanStories, sn)
		}
	}

	for epicKey := range storiesByEpic {
		for i := range storiesByEpic[epicKey] {
			storyMap[storiesByEpic[epicKey][i].issue.Key] = &storiesByEpic[epicKey][i]
		}
	}
	for i := range orphanStories {
		storyMap[orphanStories[i].issue.Key] = &orphanStories[i]
	}

	// Group tasks by story
	skippedTasks := 0
	for _, t := range tasks {
		parentKey := ""
		if p := t.Fields.Parent; p != nil {
			if _, ok := storyMap[p.Key]; ok {
				parentKey = p.Key
			}
		}
		if parentKey == "" {
			for _, link := range t.Fields.IssueLinks {
				if link.InwardIssue != nil {
					if _, ok := storyMap[link.InwardIssue.Key]; ok {
						parentKey = link.InwardIssue.Key
						break
					}
				}
				if link.OutwardIssue != nil {
					if _, ok := storyMap[link.OutwardIssue.Key]; ok {
						parentKey = link.OutwardIssue.Key
						break
					}
				}
			}
		}
		if parentKey != "" {
			if sn, ok := storyMap[parentKey]; ok {
				sn.tasks = append(sn.tasks, t)
			}
		} else {
			skippedTasks++
		}
	}

	// 5. Build MS Project task list
	var msTskList []msTask
	uid := 1
	id := 1

	now := time.Now()
	projectStart := now
	projectFinish := now

	updateBounds := func(dateStr string) {
		if dateStr == "" {
			return
		}
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return
		}
		if t.Before(projectStart) {
			projectStart = t
		}
		if t.After(projectFinish) {
			projectFinish = t
		}
	}

	estimateWorkDays := func(seconds int) int {
		if seconds <= 0 {
			return 1
		}
		days := seconds / (8 * 3600)
		if days < 1 {
			days = 1
		}
		return days
	}

	formatDuration := func(seconds int) string {
		if seconds <= 0 {
			return "PT8H0M0S"
		}
		hours := seconds / 3600
		minutes := (seconds % 3600) / 60
		return fmt.Sprintf("PT%dH%dM0S", hours, minutes)
	}

	subtractWorkDays := func(from time.Time, workDays int) time.Time {
		t := from
		for workDays > 0 {
			t = t.AddDate(0, 0, -1)
			wd := t.Weekday()
			if wd != time.Saturday && wd != time.Sunday {
				workDays--
			}
		}
		return t
	}

	issueEstimate := func(issue models.Issue) int {
		if issue.Fields.TimeTracking != nil {
			return issue.Fields.TimeTracking.OriginalEstimateSeconds
		}
		return 0
	}

	addIssueTask := func(issue models.Issue, outlineLevel int, isSummary bool) {
		summary := 0
		if isSummary {
			summary = 1
		}

		estimate := issueEstimate(issue)
		dueDate := issue.Fields.DueDate
		updateBounds(dueDate)

		task := msTask{
			UID:          uid,
			ID:           id,
			Name:         fmt.Sprintf("[%s] %s", issue.Key, issue.Fields.Summary),
			OutlineLevel: outlineLevel,
			Summary:      summary,
			Notes:        fmt.Sprintf("Статус: %s", issue.Fields.Status.Name),
		}

		if !isSummary && dueDate != "" {
			finishDate, _ := time.Parse("2006-01-02", dueDate)
			task.Finish = dueDate + "T17:00:00"
			task.Duration = formatDuration(estimate)
			startDate := subtractWorkDays(finishDate, estimateWorkDays(estimate))
			startStr := startDate.Format("2006-01-02")
			task.Start = startStr + "T08:00:00"
			task.Manual = 1
			task.DurationFormat = 7
			task.ConstraintType = 2
			task.ConstraintDate = startStr + "T08:00:00"
			updateBounds(startStr)
		}

		msTskList = append(msTskList, task)
		uid++
		id++
	}

	// Sort epics by key
	sort.Slice(epics, func(i, j int) bool {
		return epics[i].Key < epics[j].Key
	})

	// Add only epics that have stories
	skippedEpics := 0
	for _, epic := range epics {
		epicStories := storiesByEpic[epic.Key]
		if len(epicStories) == 0 {
			skippedEpics++
			continue
		}

		addIssueTask(epic, 1, true)

		sort.Slice(epicStories, func(i, j int) bool {
			return epicStories[i].issue.Key < epicStories[j].issue.Key
		})

		for _, sn := range epicStories {
			addIssueTask(sn.issue, 2, len(sn.tasks) > 0)

			sort.Slice(sn.tasks, func(i, j int) bool {
				return sn.tasks[i].Key < sn.tasks[j].Key
			})

			for _, t := range sn.tasks {
				addIssueTask(t, 3, false)
			}
		}
	}

	// 6. Generate XML
	workingDay := func(dayType int) msWeekDay {
		return msWeekDay{
			DayType:    dayType,
			DayWorking: 1,
			WorkingTimes: &msWorkingTimes{
				WorkingTime: []msWorkingTime{
					{FromTime: "08:00:00", ToTime: "12:00:00"},
					{FromTime: "13:00:00", ToTime: "17:00:00"},
				},
			},
		}
	}

	proj := msProjectXML{
		Xmlns:             "http://schemas.microsoft.com/project",
		SaveVersion:       14,
		Name:              project,
		ScheduleFromStart: 1,
		StartDate:         projectStart.Format("2006-01-02") + "T08:00:00",
		FinishDate:        projectFinish.Format("2006-01-02") + "T17:00:00",
		CalendarUID:       1,
		DefaultStartTime:  "08:00:00",
		DefaultFinishTime: "17:00:00",
		MinutesPerDay:     480,
		MinutesPerWeek:    2400,
		DaysPerMonth:      20,
		HonorConstraints:  1,
		Calendars: msCalendars{
			Calendar: []msCalendar{
				{
					UID:            1,
					Name:           "Standard",
					IsBaseCalendar: 1,
					WeekDays: msWeekDays{
						WeekDay: []msWeekDay{
							{DayType: 1, DayWorking: 0},
							workingDay(2),
							workingDay(3),
							workingDay(4),
							workingDay(5),
							workingDay(6),
							{DayType: 7, DayWorking: 0},
						},
					},
				},
			},
		},
		Tasks: msTasks{Task: msTskList},
	}

	output, err := xml.MarshalIndent(proj, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка формирования XML: %w", err)
	}

	xmlContent := xml.Header + string(output)
	fileName := project + ".xml"

	out.Printf("")
	out.Printf("Экспорт %s завершён", project)
	out.Printf("  Эпиков: %d, Историй: %d, Задач: %d", len(epics), len(storyNodes), len(tasks))
	out.Printf("  Всего элементов в файле: %d", len(msTskList))
	if skippedEpics > 0 {
		out.Printf("  Пропущено эпиков без историй: %d", skippedEpics)
	}
	if len(orphanStories) > 0 {
		out.Printf("  Пропущено историй без эпика: %d", len(orphanStories))
	}
	if skippedTasks > 0 {
		out.Printf("  Пропущено задач без истории: %d", skippedTasks)
	}

	// Send file for download
	out.SendFile(fileName, xmlContent)

	return nil
}
