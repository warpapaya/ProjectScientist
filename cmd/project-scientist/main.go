package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

type app struct {
	store *lab.Store
	tmpl  *template.Template
}

type pageData struct {
	Scope          lab.Scope
	Clients        []lab.Client
	Sites          []lab.Site
	Contacts       []lab.Contact
	ContactRoles   []lab.ContactRole
	Projects       []lab.Project
	ClientDefaults []lab.ClientDefaults
	Samples        []lab.Sample
	Audit          []lab.AuditEvent
}

func main() {
	if err := run(os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) < 2 || args[1] == "serve" {
		return serve()
	}
	switch args[1] {
	case "audit":
		if len(args) >= 3 && args[2] == "verify" {
			return auditVerify(args[3:], stdout, stderr)
		}
	case "db":
		if len(args) >= 3 {
			switch args[2] {
			case "migrate":
				return dbMigrate(args[3:], stdout, stderr)
			case "status":
				return dbStatus(args[3:], stdout, stderr)
			}
		}
	case "seed":
		return seedDB(args[2:], stdout, stderr)
	case "reset":
		return resetDB(args[2:], stdout, stderr)
	case "backup":
		return backupDB(args[2:], stdout, stderr)
	case "restore":
		return restoreDB(args[2:], stdout, stderr)
	case "smoke":
		return smokeHTTP(args[2:], stdout, stderr)
	}
	return fmt.Errorf("unknown command; supported: serve, audit verify, db migrate, db status, seed, reset, backup, restore, smoke")
}

func serve() error {
	dataDir := getenv("PSC_DATA_DIR", "/data")
	addr := getenv("PSC_ADDR", ":8080")
	store, err := lab.OpenSQLiteStore(filepath.Join(dataDir, "project-scientist.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	application := &app{store: store, tmpl: template.Must(template.ParseFiles("web/templates/index.html"))}
	mux := http.NewServeMux()
	mux.HandleFunc("/", application.index)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/state", application.apiState)
	mux.HandleFunc("POST /api/clients", application.createClient)
	mux.HandleFunc("POST /api/sites", application.createSite)
	mux.HandleFunc("POST /api/contacts", application.createContact)
	mux.HandleFunc("POST /api/contact-roles", application.assignContactRole)
	mux.HandleFunc("POST /api/projects", application.createProject)
	mux.HandleFunc("POST /api/client-defaults", application.upsertClientDefaults)
	mux.HandleFunc("POST /api/samples", application.createSample)
	mux.HandleFunc("POST /api/samples/", application.transitionSample)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	log.Printf("Project Scientist listening on %s", addr)
	return http.ListenAndServe(addr, securityHeaders(mux))
}

func auditVerify(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("audit verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", defaultDBPath(), "SQLite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := lab.OpenSQLiteStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	fmt.Fprintf(stdout, "audit verify ok db=%s\n", *dbPath)
	return nil
}

func dbMigrate(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("db migrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", defaultDBPath(), "SQLite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := lab.OpenSQLiteStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	status, err := sqliteStatus(*dbPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "db migrate ok db=%s schema_version=%d\n", *dbPath, status.SchemaVersion)
	return nil
}

type dbCounts struct {
	SchemaVersion int
	Clients       int
	Samples       int
	AuditEvents   int
}

func dbStatus(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("db status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", defaultDBPath(), "SQLite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	status, err := sqliteStatus(*dbPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "db status db=%s schema_version=%d clients=%d samples=%d audit_events=%d\n", *dbPath, status.SchemaVersion, status.Clients, status.Samples, status.AuditEvents)
	return nil
}

func sqliteStatus(dbPath string) (dbCounts, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_query_only=1")
	if err != nil {
		return dbCounts{}, err
	}
	defer db.Close()
	var status dbCounts
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&status.SchemaVersion); err != nil {
		return dbCounts{}, err
	}
	for _, item := range []struct {
		name string
		dest *int
	}{
		{"clients", &status.Clients},
		{"samples", &status.Samples},
		{"audit_events", &status.AuditEvents},
	} {
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + item.name).Scan(item.dest); err != nil {
			return dbCounts{}, err
		}
	}
	return status, nil
}

func seedDB(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", defaultDBPath(), "SQLite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := lab.OpenSQLiteStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	client, err := store.CreateClient("Clearline Synthetic Lab", "synthetic@example.test", "psc-operator-seed")
	if err != nil {
		return err
	}
	if _, err := store.CreateSample(lab.CreateSampleInput{ClientID: client.ID, Project: "Synthetic Drinking Water Compliance", Matrix: "Water", Tests: []string{"pH", "Turbidity", "Lead"}}, "psc-operator-seed"); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "seed ok db=%s client_id=%s\n", *dbPath, client.ID)
	return nil
}

func resetDB(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("reset", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", defaultDBPath(), "SQLite database path")
	force := fs.Bool("force", false, "required to reset local database")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*force {
		return errors.New("reset requires --force and is only for local lab-test data")
	}
	if err := removeSQLiteFiles(*dbPath); err != nil {
		return err
	}
	store, err := lab.OpenSQLiteStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	fmt.Fprintf(stdout, "reset ok db=%s\n", *dbPath)
	return nil
}

func backupDB(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", defaultDBPath(), "SQLite database path")
	outPath := fs.String("out", "", "backup SQLite file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*outPath) == "" {
		return errors.New("backup requires --out")
	}
	store, err := lab.OpenSQLiteStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		return err
	}
	quoted := strings.ReplaceAll(*outPath, "'", "''")
	if _, err := store.DB().Exec(`VACUUM INTO '` + quoted + `'`); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "backup ok db=%s out=%s\n", *dbPath, *outPath)
	return nil
}

func restoreDB(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", defaultDBPath(), "SQLite database path")
	backupPath := fs.String("backup", "", "backup SQLite file path")
	force := fs.Bool("force", false, "required to overwrite local database")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*backupPath) == "" {
		return errors.New("restore requires --backup")
	}
	if !*force {
		return errors.New("restore requires --force and is only for local lab-test data")
	}
	if err := verifySQLiteBackup(*backupPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		return err
	}
	if err := removeSQLiteFiles(*dbPath); err != nil {
		return err
	}
	if err := copyFile(*backupPath, *dbPath); err != nil {
		return err
	}
	store, err := lab.OpenSQLiteStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	fmt.Fprintf(stdout, "restore ok db=%s backup=%s\n", *dbPath, *backupPath)
	return nil
}

func verifySQLiteBackup(path string) error {
	store, err := lab.OpenSQLiteStore(path)
	if err != nil {
		return fmt.Errorf("backup verification failed: %w", err)
	}
	return store.Close()
}

func removeSQLiteFiles(dbPath string) error {
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func smokeHTTP(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("smoke", flag.ContinueOnError)
	fs.SetOutput(stderr)
	baseURL := fs.String("base-url", getenv("DEV_BASE_URL", "http://127.0.0.1:8097"), "running Project Scientist base URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	client := http.Client{Timeout: 5 * time.Second}
	if err := expectHTTP(&client, strings.TrimRight(*baseURL, "/")+"/healthz", "ok"); err != nil {
		return err
	}
	if err := expectHTTP(&client, strings.TrimRight(*baseURL, "/")+"/api/state", ""); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "smoke ok base_url=%s\n", *baseURL)
	return nil
}

func expectHTTP(client *http.Client, url, bodyContains string) error {
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s: %s", url, res.Status, strings.TrimSpace(string(body)))
	}
	if bodyContains != "" && !strings.Contains(string(body), bodyContains) {
		return fmt.Errorf("%s response missing %q: %s", url, bodyContains, strings.TrimSpace(string(body)))
	}
	return nil
}

func defaultDBPath() string {
	return filepath.Join(getenv("PSC_DATA_DIR", "data"), "project-scientist.db")
}

func (a *app) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	scope := scopeFromRequest(r)
	if err := a.tmpl.Execute(w, a.pageData(scope, 20)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) apiState(w http.ResponseWriter, r *http.Request) {
	scope := scopeFromRequest(r)
	writeJSON(w, a.pageData(scope, 50), http.StatusOK)
}

func (a *app) pageData(scope lab.Scope, auditLimit int) pageData {
	audit, _ := a.store.AuditEventsForScope(scope, auditLimit)
	return pageData{
		Scope:          scope,
		Clients:        a.store.ClientsForScope(scope),
		Sites:          a.store.SitesForScope(scope),
		Contacts:       a.store.ContactsForScope(scope),
		ContactRoles:   a.store.ContactRolesForScope(scope),
		Projects:       a.store.ProjectsForScope(scope),
		ClientDefaults: a.store.AllClientDefaultsForScope(scope),
		Samples:        a.store.SamplesForScope(scope),
		Audit:          audit,
	}
}

func (a *app) createClient(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	client, err := a.store.CreateClientForScope(scopeFromRequest(r), r.FormValue("name"), r.FormValue("email"), actor(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, client, http.StatusCreated)
		return
	}
	http.Redirect(w, r, scopedHome(scopeFromRequest(r)), http.StatusSeeOther)
}

func (a *app) createSite(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	site, err := a.store.CreateSiteForScope(scopeFromRequest(r), lab.SiteInput{ClientID: r.FormValue("client_id"), Name: r.FormValue("name"), Division: r.FormValue("division"), Address: r.FormValue("address")}, actor(r))
	writeMutationResponse(w, r, site, err)
}

func (a *app) createContact(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	contact, err := a.store.CreateContactForScope(scopeFromRequest(r), lab.ContactInput{ClientID: r.FormValue("client_id"), SiteID: r.FormValue("site_id"), Name: r.FormValue("name"), Email: r.FormValue("email"), Phone: r.FormValue("phone")}, actor(r))
	writeMutationResponse(w, r, contact, err)
}

func (a *app) assignContactRole(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	role, err := a.store.AssignContactRoleForScope(scopeFromRequest(r), lab.ContactRoleInput{ContactID: r.FormValue("contact_id"), Role: r.FormValue("role")}, actor(r))
	writeMutationResponse(w, r, role, err)
}

func (a *app) createProject(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	project, err := a.store.CreateProjectForScope(scopeFromRequest(r), lab.ProjectInput{ClientID: r.FormValue("client_id"), SiteID: r.FormValue("site_id"), Name: r.FormValue("name"), WorkOrder: r.FormValue("work_order"), DefaultMatrix: r.FormValue("default_matrix"), DefaultTests: splitTests(r.FormValue("default_tests"))}, actor(r))
	writeMutationResponse(w, r, project, err)
}

func (a *app) upsertClientDefaults(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defaults, err := a.store.UpsertClientDefaultsForScope(scopeFromRequest(r), lab.ClientDefaultsInput{ClientID: r.FormValue("client_id"), ReportTemplate: r.FormValue("report_template"), InvoiceEmail: r.FormValue("invoice_email"), DefaultMatrix: r.FormValue("default_matrix"), DefaultTests: splitTests(r.FormValue("default_tests"))}, actor(r))
	writeMutationResponse(w, r, defaults, err)
}

func writeMutationResponse(w http.ResponseWriter, r *http.Request, value any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, value, http.StatusCreated)
		return
	}
	http.Redirect(w, r, scopedHome(scopeFromRequest(r)), http.StatusSeeOther)
}

func (a *app) createSample(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	input := lab.CreateSampleInput{ClientID: r.FormValue("client_id"), Project: r.FormValue("project"), Matrix: r.FormValue("matrix"), Tests: splitTests(r.FormValue("tests"))}
	sample, err := a.store.CreateSampleForScope(scopeFromRequest(r), input, actor(r))
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
	if err := a.store.TransitionSampleForScope(scopeFromRequest(r), sampleID, lab.SampleStatus(r.FormValue("status")), actor(r)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
		return
	}
	http.Redirect(w, r, scopedHome(scopeFromRequest(r)), http.StatusSeeOther)
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

func actor(r *http.Request) lab.ActorContext {
	requestID := strings.TrimSpace(r.Header.Get("X-PSC-Request-ID"))
	if requestID == "" {
		requestID = "local-http-request"
	}
	return lab.MustActorContext(lab.ActorContextInput{
		UserID:            "lab-dev",
		DisplayName:       "lab-dev",
		AuthProvider:      "local-dev",
		RequestID:         requestID,
		CorrelationID:     requestID,
		TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID}},
	})
}

func scopeFromRequest(r *http.Request) lab.Scope {
	scope := lab.DefaultScope
	if tenantID := strings.TrimSpace(r.Header.Get("X-PSC-Tenant-ID")); tenantID != "" {
		scope.TenantID = tenantID
	} else if tenantID := strings.TrimSpace(r.FormValue("tenant_id")); tenantID != "" {
		scope.TenantID = tenantID
	} else if tenantID := strings.TrimSpace(r.URL.Query().Get("tenant_id")); tenantID != "" {
		scope.TenantID = tenantID
	}
	if labID := strings.TrimSpace(r.Header.Get("X-PSC-Lab-ID")); labID != "" {
		scope.LabID = labID
	} else if labID := strings.TrimSpace(r.FormValue("lab_id")); labID != "" {
		scope.LabID = labID
	} else if labID := strings.TrimSpace(r.URL.Query().Get("lab_id")); labID != "" {
		scope.LabID = labID
	}
	return scope
}

func scopedHome(scope lab.Scope) string {
	values := url.Values{}
	values.Set("tenant_id", scope.TenantID)
	values.Set("lab_id", scope.LabID)
	return "/?" + values.Encode()
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
