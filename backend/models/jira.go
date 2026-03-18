package models

import "encoding/json"

type JiraConfig struct {
	URL      string   `json:"url"`
	Login    string   `json:"login"`
	Password string   `json:"password"`
	Users    []string `json:"users"`
}

type SearchResult struct {
	StartAt    int     `json:"startAt"`
	MaxResults int     `json:"maxResults"`
	Total      int     `json:"total"`
	Issues     []Issue `json:"issues"`
}

type Issue struct {
	Key    string      `json:"key"`
	Fields IssueFields `json:"fields"`
}

type User struct {
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
}

type TimeTracking struct {
	OriginalEstimateSeconds int `json:"originalEstimateSeconds"`
	TimeSpentSeconds        int `json:"timeSpentSeconds"`
}

type Priority struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type IssueFields struct {
	Summary      string           `json:"summary"`
	Status       Status           `json:"status"`
	Priority     *Priority        `json:"priority"`
	Creator      *User            `json:"creator"`
	Assignee     *User            `json:"assignee"`
	Parent       *ParentRef       `json:"parent"`
	IssueLinks   []IssueLink      `json:"issuelinks"`
	TimeTracking *TimeTracking    `json:"timetracking"`
	DueDate      string           `json:"duedate"`
	Worklog      *WorklogResponse `json:"worklog"`
	IssueType    IssueType        `json:"issuetype"`
}

type Status struct {
	Name string `json:"name"`
}

type IssueType struct {
	Name string `json:"name"`
}

type ParentRef struct {
	Key    string          `json:"key"`
	Fields ParentRefFields `json:"fields"`
}

type ParentRefFields struct {
	IssueType IssueType `json:"issuetype"`
}

type IssueLinkType struct {
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

type IssueLink struct {
	ID           string        `json:"id"`
	Type         IssueLinkType `json:"type"`
	InwardIssue  *LinkedIssue  `json:"inwardIssue"`
	OutwardIssue *LinkedIssue  `json:"outwardIssue"`
}

type LinkedIssue struct {
	Key    string            `json:"key"`
	Fields LinkedIssueFields `json:"fields"`
}

type LinkedIssueFields struct {
	IssueType IssueType `json:"issuetype"`
}

type RemoteLink struct {
	ID           int    `json:"id"`
	Relationship string `json:"relationship"`
	Object       struct {
		URL   string `json:"url"`
		Title string `json:"title"`
		Icon  struct {
			Title string `json:"title"`
		} `json:"icon"`
	} `json:"object"`
}

type WorklogResponse struct {
	StartAt    int       `json:"startAt"`
	MaxResults int       `json:"maxResults"`
	Total      int       `json:"total"`
	Worklogs   []Worklog `json:"worklogs"`
}

type Worklog struct {
	Author           *User `json:"author"`
	TimeSpentSeconds int   `json:"timeSpentSeconds"`
}

// RawSearchResult is used for parsing raw JSON fields (e.g. epic link custom field)
type RawSearchResult struct {
	StartAt    int `json:"startAt"`
	MaxResults int `json:"maxResults"`
	Total      int `json:"total"`
	Issues     []struct {
		Key    string                     `json:"key"`
		Fields map[string]json.RawMessage `json:"fields"`
	} `json:"issues"`
}
