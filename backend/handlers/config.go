package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

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
	// Redact password
	if _, ok := cfg["jira_password"]; ok {
		cfg["jira_password"] = "***"
	}
	writeJSON(w, cfg)
}

func (h *ConfigHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}
	for _, key := range []string{"jira_url", "jira_login", "jira_password"} {
		if val, ok := body[key]; ok {
			if key == "jira_password" && val == "***" {
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

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
