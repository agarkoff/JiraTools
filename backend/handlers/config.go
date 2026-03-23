package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"jira-tools-web/gitlab"
	"jira-tools-web/jira"
	"jira-tools-web/models"
)

type ConfigHandler struct {
	DB *sql.DB
}

func (h *ConfigHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := models.GetConfig(h.DB)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// Redact secrets
	if _, ok := cfg["jira_password"]; ok {
		cfg["jira_password"] = "***"
	}
	if _, ok := cfg["gitlab_token"]; ok {
		cfg["gitlab_token"] = "***"
	}
	writeJSON(w, cfg)
}

func (h *ConfigHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}
	for _, key := range []string{"jira_url", "jira_login", "jira_password", "ollama_url", "ollama_model", "gitlab_url", "gitlab_token", "gitlab_project"} {
		if val, ok := body[key]; ok {
			if (key == "jira_password" || key == "gitlab_token") && val == "***" {
				continue // don't overwrite with redacted value
			}
			if err := models.SetConfig(h.DB, key, val); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *ConfigHandler) GetUsers(w http.ResponseWriter, r *http.Request) {
	users, err := models.GetUsers(h.DB)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if users == nil {
		users = []string{}
	}
	writeJSON(w, users)
}

func (h *ConfigHandler) AddUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Login == "" {
		http.Error(w, "invalid json or empty login", 400)
		return
	}
	if err := models.AddUser(h.DB, body.Login); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(201)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *ConfigHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	login := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if login == "" {
		http.Error(w, "login required", 400)
		return
	}
	if err := models.DeleteUser(h.DB, login); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *ConfigHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	cfg, err := models.LoadJiraConfig(h.DB)
	if err != nil {
		writeJSON(w, map[string]string{"status": "error", "message": err.Error()})
		return
	}
	if cfg.URL == "" {
		writeJSON(w, map[string]string{"status": "error", "message": "URL Jira не настроен"})
		return
	}
	if err := jira.TestConnection(cfg); err != nil {
		writeJSON(w, map[string]string{"status": "error", "message": err.Error()})
		return
	}
	writeJSON(w, map[string]string{"status": "ok", "message": "Подключение успешно"})
}

func (h *ConfigHandler) TestGitLab(w http.ResponseWriter, r *http.Request) {
	glCfg, err := models.LoadGitLabConfig(h.DB)
	if err != nil {
		writeJSON(w, map[string]string{"status": "error", "message": err.Error()})
		return
	}
	if glCfg.URL == "" {
		writeJSON(w, map[string]string{"status": "error", "message": "URL GitLab не настроен"})
		return
	}
	glc := gitlab.Config{URL: glCfg.URL, Token: glCfg.Token, Project: glCfg.Project}
	if err := gitlab.TestConnection(glc); err != nil {
		writeJSON(w, map[string]string{"status": "error", "message": err.Error()})
		return
	}
	writeJSON(w, map[string]string{"status": "ok", "message": "GitLab подключение успешно"})
}

func (h *ConfigHandler) GetFnParams(w http.ResponseWriter, r *http.Request) {
	funcID := strings.TrimPrefix(r.URL.Path, "/api/fn-params/")
	if funcID == "" {
		http.Error(w, "function id required", 400)
		return
	}
	cfg, err := models.GetConfig(h.DB)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	key := "fn-params-" + funcID
	val, ok := cfg[key]
	if !ok {
		writeJSON(w, map[string]string{})
		return
	}
	// val is JSON string, write it raw
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(val))
}

func (h *ConfigHandler) SetFnParams(w http.ResponseWriter, r *http.Request) {
	funcID := strings.TrimPrefix(r.URL.Path, "/api/fn-params/")
	if funcID == "" {
		http.Error(w, "function id required", 400)
		return
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}
	raw, _ := json.Marshal(body)
	if err := models.SetConfig(h.DB, "fn-params-"+funcID, string(raw)); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
