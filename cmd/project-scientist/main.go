package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

type app struct {
	store *lab.Store
	tmpl  *template.Template
}

type pageData struct {
	Clients []lab.Client
	Samples []lab.Sample
	Audit   []lab.AuditEvent
}

func main() {
	dataDir := getenv("PSC_DATA_DIR", "/data")
	addr := getenv("PSC_ADDR", ":8080")
	store, err := lab.OpenSQLiteStore(filepath.Join(dataDir, "project-scientist.db"))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	application := &app{store: store, tmpl: template.Must(template.ParseFiles("web/templates/index.html"))}
	mux := http.NewServeMux()
	mux.HandleFunc("/", application.index)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/state", application.apiState)
	mux.HandleFunc("POST /api/clients", application.createClient)
	mux.HandleFunc("POST /api/samples", application.createSample)
	mux.HandleFunc("POST /api/samples/", application.transitionSample)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	log.Printf("Project Scientist listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, securityHeaders(mux)))
}

func (a *app) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	audit, _ := a.store.AuditEvents(20)
	if err := a.tmpl.Execute(w, pageData{Clients: a.store.Clients(), Samples: a.store.Samples(), Audit: audit}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) apiState(w http.ResponseWriter, r *http.Request) {
	audit, _ := a.store.AuditEvents(50)
	writeJSON(w, pageData{Clients: a.store.Clients(), Samples: a.store.Samples(), Audit: audit}, http.StatusOK)
}

func (a *app) createClient(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	client, err := a.store.CreateClient(r.FormValue("name"), r.FormValue("email"), actor(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, client, http.StatusCreated)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *app) createSample(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	input := lab.CreateSampleInput{ClientID: r.FormValue("client_id"), Project: r.FormValue("project"), Matrix: r.FormValue("matrix"), Tests: splitTests(r.FormValue("tests"))}
	sample, err := a.store.CreateSample(input, actor(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, sample, http.StatusCreated)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *app) transitionSample(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/transition") {
		http.NotFound(w, r)
		return
	}
	sampleID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/samples/"), "/transition")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.store.TransitionSample(sampleID, lab.SampleStatus(r.FormValue("status")), actor(r)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func splitTests(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' })
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func actor(r *http.Request) string {
	if actor := strings.TrimSpace(r.Header.Get("X-PSC-Actor")); actor != "" {
		return actor
	}
	if actor := strings.TrimSpace(r.FormValue("actor")); actor != "" {
		return actor
	}
	return "lab-dev"
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func writeJSON(w http.ResponseWriter, value any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
