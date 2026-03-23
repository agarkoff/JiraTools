package gitlab

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	URL     string
	Token   string
	Project string // project ID or URL-encoded path
}

type MergeRequest struct {
	IID          int    `json:"iid"`
	Title        string `json:"title"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	WebURL       string `json:"web_url"`
	MergedAt     string `json:"merged_at"`
	Author       struct {
		Name string `json:"name"`
	} `json:"author"`
}

type Commit struct {
	ID         string `json:"id"`
	ShortID    string `json:"short_id"`
	Title      string `json:"title"`
	Message    string `json:"message"`
	AuthorName string `json:"author_name"`
	CreatedAt  string `json:"created_at"`
}

type DiffFile struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	Diff    string `json:"diff"`
}

type Branch struct {
	Name string `json:"name"`
}

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

func apiGet(cfg Config, path string, params url.Values) ([]byte, error) {
	u := strings.TrimRight(cfg.URL, "/") + "/api/v4/projects/" +
		url.PathEscape(cfg.Project) + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", cfg.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gitlab read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// SearchMergeRequests finds MRs matching a search string (title, branch).
func SearchMergeRequests(cfg Config, search string) ([]MergeRequest, error) {
	params := url.Values{}
	params.Set("search", search)
	params.Set("per_page", "50")
	params.Set("state", "all")

	body, err := apiGet(cfg, "/merge_requests", params)
	if err != nil {
		return nil, err
	}
	var mrs []MergeRequest
	return mrs, json.Unmarshal(body, &mrs)
}

// SearchCommits finds commits on a branch containing the search string in message.
func SearchCommits(cfg Config, branch, search string) ([]Commit, error) {
	params := url.Values{}
	params.Set("ref_name", branch)
	params.Set("search", search)
	params.Set("per_page", "100")

	body, err := apiGet(cfg, "/repository/commits", params)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	return commits, json.Unmarshal(body, &commits)
}

// GetCommitDiff returns the diff files for a commit.
func GetCommitDiff(cfg Config, sha string) ([]DiffFile, error) {
	body, err := apiGet(cfg, "/repository/commits/"+url.PathEscape(sha)+"/diff", nil)
	if err != nil {
		return nil, err
	}
	var diffs []DiffFile
	return diffs, json.Unmarshal(body, &diffs)
}

// ListBranches returns branches matching a search prefix.
func ListBranches(cfg Config, search string) ([]Branch, error) {
	params := url.Values{}
	params.Set("search", search)
	params.Set("per_page", "100")

	body, err := apiGet(cfg, "/repository/branches", params)
	if err != nil {
		return nil, err
	}
	var branches []Branch
	return branches, json.Unmarshal(body, &branches)
}

// TestConnection checks that the API token and project are valid.
func TestConnection(cfg Config) error {
	u := strings.TrimRight(cfg.URL, "/") + "/api/v4/projects/" + url.PathEscape(cfg.Project)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", cfg.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка подключения: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitLab %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
