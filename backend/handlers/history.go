package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"jira-tools-web/models"
)

type HistoryHandler struct {
	DB *sql.DB
}

func (h *HistoryHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0
	function := ""

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}
	if v := r.URL.Query().Get("function"); v != "" {
		function = v
	}

	runs, err := models.GetRuns(h.DB, limit, offset, function)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if runs == nil {
		runs = []models.Run{}
	}
	writeJSON(w, runs)
}

func (h *HistoryHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	// Remove any trailing parts like /output
	if idx := strings.Index(idStr, "/"); idx >= 0 {
		idStr = idStr[:idx]
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}

	run, err := models.GetRun(h.DB, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if run == nil {
		http.Error(w, "not found", 404)
		return
	}
	writeJSON(w, run)
}

func (h *HistoryHandler) GetRunOutput(w http.ResponseWriter, r *http.Request) {
	// Path: /api/runs/{id}/output
	path := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	idStr := strings.TrimSuffix(path, "/output")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}

	lines, err := models.GetRunOutput(h.DB, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if lines == nil {
		lines = []models.RunOutputLine{}
	}
	writeJSON(w, lines)
}

func (h *HistoryHandler) DeleteRun(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}

	if err := models.DeleteRun(h.DB, id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}
