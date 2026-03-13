package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"jira-tools-web/functions"
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

type FunctionsHandler struct {
	DB *sql.DB
}

func (h *FunctionsHandler) ListFunctions(w http.ResponseWriter, r *http.Request) {
	registry := functions.GetRegistry()
	writeJSON(w, registry)
}

func (h *FunctionsHandler) RunFunction(w http.ResponseWriter, r *http.Request) {
	// Extract function name from path: /api/functions/{name}/run
	path := strings.TrimPrefix(r.URL.Path, "/api/functions/")
	name := strings.TrimSuffix(path, "/run")

	// Find function in registry
	var funcDef *functions.FuncDef
	for _, f := range functions.GetRegistry() {
		if f.ID == name {
			funcDef = &f
			break
		}
	}
	if funcDef == nil {
		http.Error(w, "unknown function: "+name, 404)
		return
	}

	// Parse parameters
	var params map[string]string
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	// Load Jira config
	cfg, err := models.LoadJiraConfig(h.DB)
	if err != nil {
		http.Error(w, "config error: "+err.Error(), 500)
		return
	}

	// Create run record
	paramsJSON, _ := json.Marshal(params)
	runID, err := models.CreateRun(h.DB, name, paramsJSON)
	if err != nil {
		http.Error(w, "db error: "+err.Error(), 500)
		return
	}

	// Setup SSE
	sse.SetupSSE(w)
	out := sse.NewWriter(w, h.DB, runID)
	out.SendStarted()

	// Run function
	if err := funcDef.Runner(cfg, params, out); err != nil {
		errMsg := err.Error()
		models.FinishRun(h.DB, runID, "error", &errMsg)
		out.SendError(errMsg)
		return
	}

	models.FinishRun(h.DB, runID, "completed", nil)
	out.SendCompleted()
}
