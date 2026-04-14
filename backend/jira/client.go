package jira

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"jira-tools-web/models"
)

var httpClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

func DoSearch(cfg models.JiraConfig, jql string, fields string, startAt int) ([]byte, error) {
	params := url.Values{}
	params.Set("jql", jql)
	params.Set("fields", fields)
	params.Set("startAt", fmt.Sprintf("%d", startAt))
	params.Set("maxResults", "100")

	reqURL := strings.TrimRight(cfg.URL, "/") + "/rest/api/2/search?" + params.Encode()

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cfg.Login, cfg.Password)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса к Jira: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Jira вернула статус %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func SearchIssues(cfg models.JiraConfig, jql string, fields string, startAt int) (*models.SearchResult, error) {
	body, err := DoSearch(cfg, jql, fields, startAt)
	if err != nil {
		return nil, err
	}
	var result models.SearchResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}
	if cfg.DemoMode {
		for i := range result.Issues {
			MaskIssue(&result.Issues[i])
		}
	}
	return &result, nil
}

func DoSearchExpand(cfg models.JiraConfig, jql string, fields string, expand string, startAt int) ([]byte, error) {
	params := url.Values{}
	params.Set("jql", jql)
	params.Set("fields", fields)
	params.Set("expand", expand)
	params.Set("startAt", fmt.Sprintf("%d", startAt))
	params.Set("maxResults", "100")

	reqURL := strings.TrimRight(cfg.URL, "/") + "/rest/api/2/search?" + params.Encode()

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cfg.Login, cfg.Password)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса к Jira: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Jira вернула статус %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func SearchIssuesDefault(cfg models.JiraConfig, jql string, startAt int) (*models.SearchResult, error) {
	return SearchIssues(cfg, jql, "key,summary,status,creator,parent,issuelinks", startAt)
}

func FetchIssueWorklogs(cfg models.JiraConfig, issueKey string) ([]models.Worklog, error) {
	reqURL := strings.TrimRight(cfg.URL, "/") + "/rest/api/2/issue/" + issueKey + "/worklog"

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cfg.Login, cfg.Password)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса ворклогов для %s: %w", issueKey, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Jira вернула статус %d для ворклогов %s", resp.StatusCode, issueKey)
	}

	var result models.WorklogResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ворклогов: %w", err)
	}
	if cfg.DemoMode {
		MaskWorklogs(result.Worklogs)
	}
	return result.Worklogs, nil
}

func GetCompleteWorklogs(issue models.Issue, cfg models.JiraConfig) []models.Worklog {
	if wl := issue.Fields.Worklog; wl != nil {
		if wl.Total <= len(wl.Worklogs) {
			return wl.Worklogs
		}
	}
	worklogs, err := FetchIssueWorklogs(cfg, issue.Key)
	if err != nil {
		if wl := issue.Fields.Worklog; wl != nil {
			return wl.Worklogs
		}
		return nil
	}
	return worklogs
}

func FetchRemoteLinks(cfg models.JiraConfig, issueKey string) ([]models.RemoteLink, error) {
	reqURL := strings.TrimRight(cfg.URL, "/") + "/rest/api/2/issue/" + issueKey + "/remotelink"

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cfg.Login, cfg.Password)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса remote links для %s: %w", issueKey, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Jira вернула статус %d для remote links %s", resp.StatusCode, issueKey)
	}

	var links []models.RemoteLink
	if err := json.Unmarshal(body, &links); err != nil {
		return nil, fmt.Errorf("ошибка парсинга remote links: %w", err)
	}
	return links, nil
}

// FetchIssueText returns description and comment bodies for an issue.
func FetchIssueText(cfg models.JiraConfig, issueKey string) (description string, comments []string, err error) {
	reqURL := strings.TrimRight(cfg.URL, "/") + "/rest/api/2/issue/" + issueKey + "?fields=description,comment"

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.SetBasicAuth(cfg.Login, cfg.Password)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("Jira вернула статус %d", resp.StatusCode)
	}

	var result struct {
		Fields struct {
			Description string `json:"description"`
			Comment     struct {
				Comments []struct {
					Body string `json:"body"`
				} `json:"comments"`
			} `json:"comment"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", nil, err
	}

	for _, c := range result.Fields.Comment.Comments {
		comments = append(comments, c.Body)
	}
	return result.Fields.Description, comments, nil
}

func UpdateIssue(cfg models.JiraConfig, issueKey string, payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	reqURL := strings.TrimRight(cfg.URL, "/") + "/rest/api/2/issue/" + issueKey
	req, err := http.NewRequest("PUT", reqURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.Login, cfg.Password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка запроса к Jira: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Jira вернула статус %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func DeleteIssueLink(cfg models.JiraConfig, linkID string) error {
	reqURL := strings.TrimRight(cfg.URL, "/") + "/rest/api/2/issueLink/" + linkID
	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.Login, cfg.Password)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("статус %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func CreateIssueLink(cfg models.JiraConfig, linkTypeName, inwardKey, outwardKey string) error {
	payload := map[string]interface{}{
		"type":         map[string]string{"name": linkTypeName},
		"inwardIssue":  map[string]string{"key": inwardKey},
		"outwardIssue": map[string]string{"key": outwardKey},
	}
	data, _ := json.Marshal(payload)

	reqURL := strings.TrimRight(cfg.URL, "/") + "/rest/api/2/issueLink"
	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.Login, cfg.Password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("статус %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func GetParentLinkType(cfg models.JiraConfig) (*models.IssueLinkType, error) {
	reqURL := strings.TrimRight(cfg.URL, "/") + "/rest/api/2/issueLinkType"
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cfg.Login, cfg.Password)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		IssueLinkTypes []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Inward  string `json:"inward"`
			Outward string `json:"outward"`
		} `json:"issueLinkTypes"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("ошибка парсинга: %w", err)
	}

	for _, lt := range result.IssueLinkTypes {
		lower := strings.ToLower(lt.Name + " " + lt.Inward + " " + lt.Outward)
		if strings.Contains(lower, "parent") {
			return &models.IssueLinkType{Name: lt.Name, Inward: lt.Inward, Outward: lt.Outward}, nil
		}
	}
	return nil, fmt.Errorf("не найден тип связи с 'parent' среди доступных типов")
}

func TestConnection(cfg models.JiraConfig) error {
	reqURL := strings.TrimRight(cfg.URL, "/") + "/rest/api/2/myself"
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.Login, cfg.Password)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка подключения: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Jira вернула статус %d", resp.StatusCode)
	}
	return nil
}
