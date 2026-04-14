package gitlab

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	URL   string
	Token string
}

type MergeRequest struct {
	IID          int    `json:"iid"`
	ProjectID    int    `json:"project_id"`
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

// ParsedLink represents a GitLab URL parsed into project path + resource.
type ParsedLink struct {
	ProjectPath string // e.g. "group/project"
	Type        string // "mr" or "commit"
	MRIID       int    // for MRs
	CommitSHA   string // for commits
}

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

func apiGetRaw(cfg Config, path string, params url.Values) ([]byte, error) {
	u := BaseHost(cfg.URL) + "/api/v4" + path
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

func projectPrefix(projectPath string) string {
	return "/projects/" + url.PathEscape(projectPath)
}

// BaseHost extracts "https://host" from a full URL, stripping any path.
func BaseHost(rawURL string) string {
	u, err := url.Parse(strings.TrimRight(rawURL, "/"))
	if err != nil || u.Host == "" {
		return strings.TrimRight(rawURL, "/")
	}
	return u.Scheme + "://" + u.Host
}

// ParseGitLabURL extracts project path and resource from a GitLab URL.
// Returns nil if the URL doesn't match the configured GitLab instance.
func ParseGitLabURL(baseURL, linkURL string) *ParsedLink {
	base := strings.ToLower(BaseHost(baseURL))
	lower := strings.ToLower(linkURL)
	if !strings.HasPrefix(lower, base) {
		return nil
	}
	rest := linkURL[len(base):]
	return parseGitLabPath(rest)
}

// FindAllGitLabURLs scans arbitrary text for GitLab URLs and returns parsed links.
func FindAllGitLabURLs(baseURL, text string) []ParsedLink {
	base := BaseHost(baseURL)
	escaped := regexp.QuoteMeta(base)
	re := regexp.MustCompile(`(?i)` + escaped + `(/[^\s"<>)\]]+)`)
	matches := re.FindAllStringSubmatch(text, -1)
	var result []ParsedLink
	for _, m := range matches {
		if p := parseGitLabPath(m[1]); p != nil {
			result = append(result, *p)
		}
	}
	return result
}

var (
	mrPathRe     = regexp.MustCompile(`^/(.+?)/-/merge_requests/(\d+)`)
	commitPathRe = regexp.MustCompile(`^/(.+?)/-/commit/([0-9a-f]+)`)
)

func parseGitLabPath(path string) *ParsedLink {
	if m := mrPathRe.FindStringSubmatch(path); m != nil {
		iid, _ := strconv.Atoi(m[2])
		return &ParsedLink{ProjectPath: m[1], Type: "mr", MRIID: iid}
	}
	if m := commitPathRe.FindStringSubmatch(path); m != nil {
		return &ParsedLink{ProjectPath: m[1], Type: "commit", CommitSHA: m[2]}
	}
	return nil
}

// GetMergeRequest fetches a single MR by project path and IID.
func GetMergeRequest(cfg Config, project string, iid int) (*MergeRequest, error) {
	path := fmt.Sprintf("%s/merge_requests/%d", projectPrefix(project), iid)
	body, err := apiGetRaw(cfg, path, nil)
	if err != nil {
		return nil, err
	}
	var mr MergeRequest
	return &mr, json.Unmarshal(body, &mr)
}

// GetMRCommits returns all commits belonging to a merge request.
func GetMRCommits(cfg Config, project string, iid int) ([]Commit, error) {
	path := fmt.Sprintf("%s/merge_requests/%d/commits", projectPrefix(project), iid)
	params := url.Values{}
	params.Set("per_page", "100")
	body, err := apiGetRaw(cfg, path, params)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	return commits, json.Unmarshal(body, &commits)
}

// GetCommit fetches a single commit.
func GetCommit(cfg Config, project string, sha string) (*Commit, error) {
	path := projectPrefix(project) + "/repository/commits/" + url.PathEscape(sha)
	body, err := apiGetRaw(cfg, path, nil)
	if err != nil {
		return nil, err
	}
	var c Commit
	return &c, json.Unmarshal(body, &c)
}

// SearchCommits finds commits on a branch containing the search string in message.
func SearchCommits(cfg Config, project string, branch, search string) ([]Commit, error) {
	params := url.Values{}
	params.Set("ref_name", branch)
	params.Set("search", search)
	params.Set("per_page", "100")

	body, err := apiGetRaw(cfg, projectPrefix(project)+"/repository/commits", params)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	return commits, json.Unmarshal(body, &commits)
}

// GetCommitDiff returns the diff files for a commit.
func GetCommitDiff(cfg Config, project string, sha string) ([]DiffFile, error) {
	body, err := apiGetRaw(cfg, projectPrefix(project)+"/repository/commits/"+url.PathEscape(sha)+"/diff", nil)
	if err != nil {
		return nil, err
	}
	var diffs []DiffFile
	return diffs, json.Unmarshal(body, &diffs)
}

// ListBranches returns branches matching a search prefix.
func ListBranches(cfg Config, project string, search string) ([]Branch, error) {
	params := url.Values{}
	params.Set("search", search)
	params.Set("per_page", "100")

	body, err := apiGetRaw(cfg, projectPrefix(project)+"/repository/branches", params)
	if err != nil {
		return nil, err
	}
	var branches []Branch
	return branches, json.Unmarshal(body, &branches)
}

// TestConnection checks that the API token is valid.
func TestConnection(cfg Config) error {
	u := BaseHost(cfg.URL) + "/api/v4/user"
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
