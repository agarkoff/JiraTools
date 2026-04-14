package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"jira-tools-web/db"
	"jira-tools-web/handlers"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	database, err := db.Connect()
	if err != nil {
		log.Fatalf("DB connection failed: %v", err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		log.Fatalf("Migrations failed: %v", err)
	}
	log.Println("Database ready")

	configH := &handlers.ConfigHandler{DB: database}
	funcH := &handlers.FunctionsHandler{DB: database}
	histH := &handlers.HistoryHandler{DB: database}

	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// Config
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			configH.GetConfig(w, r)
		case "PUT":
			configH.UpdateConfig(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/config/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			configH.TestConnection(w, r)
		} else {
			http.Error(w, "method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/config/test-gitlab", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			configH.TestGitLab(w, r)
		} else {
			http.Error(w, "method not allowed", 405)
		}
	})

	// Users
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			configH.GetUsers(w, r)
		case "POST":
			configH.AddUser(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			configH.DeleteUser(w, r)
		} else {
			http.Error(w, "method not allowed", 405)
		}
	})

	// Vacations
	mux.HandleFunc("/api/vacations", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			configH.GetVacations(w, r)
		case "POST":
			configH.AddVacation(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/vacations/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			configH.DeleteVacation(w, r)
		} else {
			http.Error(w, "method not allowed", 405)
		}
	})

	// Resolved commits
	mux.HandleFunc("/api/resolved-commits", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			configH.ResolveCommit(w, r)
		case "DELETE":
			configH.UnresolveCommit(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})

	// Function params (saved state)
	mux.HandleFunc("/api/fn-params/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			configH.GetFnParams(w, r)
		case "PUT":
			configH.SetFnParams(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})

	// Functions
	mux.HandleFunc("/api/functions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			funcH.ListFunctions(w, r)
		} else {
			http.Error(w, "method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/functions/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/run"):
			funcH.RunFunction(w, r)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/latest"):
			funcH.GetLatestResult(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})

	// Runs history
	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			histH.ListRuns(w, r)
		} else {
			http.Error(w, "method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/runs/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/output"):
			histH.GetRunOutput(w, r)
		case r.Method == "GET":
			histH.GetRun(w, r)
		case r.Method == "DELETE":
			histH.DeleteRun(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}
