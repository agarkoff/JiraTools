package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Config DB operations

func GetConfig(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM config")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cfg := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		cfg[k] = v
	}
	return cfg, nil
}

func SetConfig(db *sql.DB, key, value string) error {
	_, err := db.Exec(`INSERT INTO config (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = $2`, key, value)
	return err
}

func LoadJiraConfig(db *sql.DB) (JiraConfig, error) {
	cfg, err := GetConfig(db)
	if err != nil {
		return JiraConfig{}, err
	}
	users, err := GetUsers(db)
	if err != nil {
		return JiraConfig{}, err
	}
	return JiraConfig{
		URL:      cfg["jira_url"],
		Login:    cfg["jira_login"],
		Password: cfg["jira_password"],
		Users:    users,
		DemoMode: cfg["demo_mode"] == "true",
	}, nil
}

type LLMConfig struct {
	URL   string
	Model string
}

func LoadLLMConfig(db *sql.DB) (LLMConfig, error) {
	cfg, err := GetConfig(db)
	if err != nil {
		return LLMConfig{}, err
	}
	return LLMConfig{
		URL:   cfg["ollama_url"],
		Model: cfg["ollama_model"],
	}, nil
}

type GitLabConfig struct {
	URL   string
	Token string
}

func LoadGitLabConfig(db *sql.DB) (GitLabConfig, error) {
	cfg, err := GetConfig(db)
	if err != nil {
		return GitLabConfig{}, err
	}
	return GitLabConfig{
		URL:   cfg["gitlab_url"],
		Token: cfg["gitlab_token"],
	}, nil
}

// Users DB operations

func GetUsers(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT login FROM users ORDER BY login")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []string
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			return nil, err
		}
		users = append(users, login)
	}
	return users, nil
}

func AddUser(db *sql.DB, login string) error {
	_, err := db.Exec("INSERT INTO users (login) VALUES ($1) ON CONFLICT DO NOTHING", login)
	return err
}

func DeleteUser(db *sql.DB, login string) error {
	_, err := db.Exec("DELETE FROM users WHERE login = $1", login)
	return err
}

// Runs DB operations

type Run struct {
	ID         int              `json:"id"`
	Function   string           `json:"function"`
	Params     json.RawMessage  `json:"params"`
	Status     string           `json:"status"`
	Error      *string          `json:"error"`
	StartedAt  time.Time        `json:"started_at"`
	FinishedAt *time.Time       `json:"finished_at"`
}

func CreateRun(db *sql.DB, function string, params json.RawMessage) (int, error) {
	var id int
	err := db.QueryRow(
		"INSERT INTO runs (function, params) VALUES ($1, $2) RETURNING id",
		function, params,
	).Scan(&id)
	return id, err
}

func FinishRun(db *sql.DB, id int, status string, errMsg *string) error {
	_, err := db.Exec(
		"UPDATE runs SET status = $1, error = $2, finished_at = NOW() WHERE id = $3",
		status, errMsg, id,
	)
	return err
}

func GetRuns(db *sql.DB, limit, offset int, function string) ([]Run, error) {
	query := "SELECT id, function, params, status, error, started_at, finished_at FROM runs"
	args := []interface{}{}
	argIdx := 1

	if function != "" {
		query += " WHERE function = $1"
		args = append(args, function)
		argIdx++
	}

	query += " ORDER BY started_at DESC"
	query += " LIMIT $" + itoa(argIdx) + " OFFSET $" + itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.Function, &r.Params, &r.Status, &r.Error, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, nil
}

func GetRun(db *sql.DB, id int) (*Run, error) {
	var r Run
	err := db.QueryRow(
		"SELECT id, function, params, status, error, started_at, finished_at FROM runs WHERE id = $1", id,
	).Scan(&r.ID, &r.Function, &r.Params, &r.Status, &r.Error, &r.StartedAt, &r.FinishedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &r, err
}

func DeleteRun(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM runs WHERE id = $1", id)
	return err
}

// Run output

type RunOutputLine struct {
	LineNum int    `json:"line_num"`
	Text    string `json:"text"`
}

func InsertRunOutput(db *sql.DB, runID, lineNum int, text string) error {
	_, err := db.Exec(
		"INSERT INTO run_output (run_id, line_num, text) VALUES ($1, $2, $3)",
		runID, lineNum, text,
	)
	return err
}

func GetRunOutput(db *sql.DB, runID int) ([]RunOutputLine, error) {
	rows, err := db.Query(
		"SELECT line_num, text FROM run_output WHERE run_id = $1 ORDER BY line_num", runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []RunOutputLine
	for rows.Next() {
		var l RunOutputLine
		if err := rows.Scan(&l.LineNum, &l.Text); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, nil
}

// Run events (structured: table, gantt, file)

type RunEvent struct {
	Seq       int             `json:"seq"`
	EventType string          `json:"type"`
	Data      json.RawMessage `json:"data"`
}

func InsertRunEvent(db *sql.DB, runID, seq int, eventType, data string) error {
	_, err := db.Exec(
		"INSERT INTO run_events (run_id, seq, event_type, data) VALUES ($1, $2, $3, $4)",
		runID, seq, eventType, data,
	)
	return err
}

func GetRunEvents(db *sql.DB, runID int) ([]RunEvent, error) {
	rows, err := db.Query(
		"SELECT seq, event_type, data FROM run_events WHERE run_id = $1 ORDER BY seq", runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []RunEvent
	for rows.Next() {
		var e RunEvent
		var dataStr string
		if err := rows.Scan(&e.Seq, &e.EventType, &dataStr); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(dataStr)
		events = append(events, e)
	}
	return events, nil
}

func GetLatestCompletedRun(db *sql.DB, function string) (*Run, error) {
	var r Run
	err := db.QueryRow(
		`SELECT id, function, params, status, error, started_at, finished_at
		 FROM runs WHERE function = $1 AND status = 'completed'
		 ORDER BY finished_at DESC LIMIT 1`, function,
	).Scan(&r.ID, &r.Function, &r.Params, &r.Status, &r.Error, &r.StartedAt, &r.FinishedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &r, err
}

// Task cache

type TaskCacheEntry struct {
	Key         string `json:"key"`
	Project     string `json:"project"`
	IssueType   string `json:"issue_type"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func UpsertTaskCache(db *sql.DB, key, project, issueType, summary, description, status string) error {
	_, err := db.Exec(`INSERT INTO task_cache (key, project, issue_type, summary, description, status, cached_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (key) DO UPDATE SET
			summary = $4, description = $5, status = $6, issue_type = $3, cached_at = NOW()`,
		key, project, issueType, summary, description, status)
	return err
}

func GetCachedTasks(db *sql.DB, projects []string) ([]TaskCacheEntry, error) {
	ph := make([]string, len(projects))
	args := make([]interface{}, len(projects))
	for i, p := range projects {
		ph[i] = fmt.Sprintf("$%d", i+1)
		args[i] = p
	}
	rows, err := db.Query(
		fmt.Sprintf(`SELECT key, project, issue_type, summary, description, status
			FROM task_cache WHERE project IN (%s) ORDER BY key`, strings.Join(ph, ",")),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []TaskCacheEntry
	for rows.Next() {
		var t TaskCacheEntry
		if err := rows.Scan(&t.Key, &t.Project, &t.IssueType, &t.Summary, &t.Description, &t.Status); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// User vacations

type Vacation struct {
	ID       int    `json:"id"`
	Login    string `json:"user_login"`
	DateFrom string `json:"date_from"`
	DateTo   string `json:"date_to"`
	Comment  string `json:"comment"`
}

func GetUserVacations(db *sql.DB, login string) ([]Vacation, error) {
	rows, err := db.Query(
		"SELECT id, user_login, date_from::text, date_to::text, comment FROM user_vacations WHERE user_login = $1 ORDER BY date_from",
		login,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Vacation
	for rows.Next() {
		var v Vacation
		if err := rows.Scan(&v.ID, &v.Login, &v.DateFrom, &v.DateTo, &v.Comment); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

func GetAllVacations(db *sql.DB) ([]Vacation, error) {
	rows, err := db.Query(
		"SELECT id, user_login, date_from::text, date_to::text, comment FROM user_vacations ORDER BY user_login, date_from",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Vacation
	for rows.Next() {
		var v Vacation
		if err := rows.Scan(&v.ID, &v.Login, &v.DateFrom, &v.DateTo, &v.Comment); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

func AddVacation(db *sql.DB, login, dateFrom, dateTo, comment string) (int, error) {
	var id int
	err := db.QueryRow(
		"INSERT INTO user_vacations (user_login, date_from, date_to, comment) VALUES ($1, $2, $3, $4) RETURNING id",
		login, dateFrom, dateTo, comment,
	).Scan(&id)
	return id, err
}

func DeleteVacation(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM user_vacations WHERE id = $1", id)
	return err
}

// Resolved commits

func GetResolvedCommits(db *sql.DB, issueKey string) (map[string]bool, error) {
	rows, err := db.Query("SELECT commit_sha FROM resolved_commits WHERE issue_key = $1", issueKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]bool)
	for rows.Next() {
		var sha string
		if err := rows.Scan(&sha); err != nil {
			return nil, err
		}
		result[sha] = true
	}
	return result, nil
}

func ResolveCommit(db *sql.DB, issueKey, commitSHA string) error {
	_, err := db.Exec(`INSERT INTO resolved_commits (issue_key, commit_sha) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		issueKey, commitSHA)
	return err
}

func UnresolveCommit(db *sql.DB, issueKey, commitSHA string) error {
	_, err := db.Exec("DELETE FROM resolved_commits WHERE issue_key = $1 AND commit_sha = $2",
		issueKey, commitSHA)
	return err
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
