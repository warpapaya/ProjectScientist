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
	"strconv"
	"strings"
	"time"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

type app struct {
	store            *lab.Store
	tmpl             *template.Template
	demoResetEnabled bool
	fixturePath      string
}

type pageData struct {
	Scope                       lab.Scope
	Clients                     []lab.Client
	Sites                       []lab.Site
	Contacts                    []lab.Contact
	ContactRoles                []lab.ContactRole
	Projects                    []lab.Project
	ClientDefaults              []lab.ClientDefaults
	Samples                     []lab.Sample
	Results                     []lab.Result
	Audit                       []lab.AuditEvent
	Departments                 []lab.CatalogDepartment
	Units                       []lab.CatalogUnit
	Methods                     []lab.CatalogMethod
	Analytes                    []lab.CatalogAnalyte
	Services                    []lab.AnalysisService
	Profiles                    []lab.AnalysisProfile
	AnalysisRequestLines        []lab.AnalysisRequestLine
	Worksheets                  []lab.Worksheet
	SampleReference             []lab.SampleReferenceItem
	MatrixReferences            []lab.SampleReferenceItem
	ContainerReferences         []lab.SampleReferenceItem
	PreservativeReferences      []lab.SampleReferenceItem
	StorageLocationReferences   []lab.SampleReferenceItem
	ReceivedConditionReferences []lab.SampleReferenceItem
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
	case "mvp":
		if len(args) >= 3 && args[2] == "vertical-slice" {
			return mvpVerticalSlice(args[3:], stdout, stderr)
		}
	case "smoke":
		return smokeHTTP(args[2:], stdout, stderr)
	}
	return fmt.Errorf("unknown command; supported: serve, audit verify, db migrate, db status, seed, reset, backup, restore, mvp vertical-slice, smoke")
}

func serve() error {
	dataDir := getenv("PSC_DATA_DIR", "/data")
	addr := getenv("PSC_ADDR", ":8080")
	store, err := lab.OpenSQLiteStore(filepath.Join(dataDir, "project-scientist.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	application := &app{
		store:            store,
		tmpl:             template.Must(template.ParseFiles("web/templates/index.html")),
		demoResetEnabled: strings.EqualFold(getenv("PSC_ENABLE_DEMO_RESET", "false"), "true"),
		fixturePath:      getenv("PSC_SYNTHETIC_FIXTURE_PATH", "/app/fixtures/mvp_synthetic_lab.json"),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", application.index)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/state", application.apiState)
	mux.HandleFunc("POST /api/demo/reset", application.demoReset)
	mux.HandleFunc("POST /api/clients", application.createClient)
	mux.HandleFunc("POST /api/sites", application.createSite)
	mux.HandleFunc("POST /api/contacts", application.createContact)
	mux.HandleFunc("POST /api/contact-roles", application.assignContactRole)
	mux.HandleFunc("POST /api/projects", application.createProject)
	mux.HandleFunc("POST /api/client-defaults", application.upsertClientDefaults)
	mux.HandleFunc("POST /api/sample-intake-templates", application.createSampleIntakeTemplate)
	mux.HandleFunc("POST /api/sample-intake-templates/", application.createSamplesFromTemplate)
	mux.HandleFunc("POST /api/results", application.createResult)
	mux.HandleFunc("POST /api/results/", application.resultAction)
	mux.HandleFunc("POST /api/samples", application.createSample)
	mux.HandleFunc("GET /api/samples/", application.sampleLabelArtifact)
	mux.HandleFunc("POST /api/samples/", application.sampleAction)
	mux.HandleFunc("POST /api/worksheets", application.createWorksheet)
	mux.HandleFunc("POST /api/worksheets/", application.routeWorksheetMutation)
	mux.HandleFunc("POST /api/catalog/departments", application.createCatalogDepartment)
	mux.HandleFunc("POST /api/catalog/units", application.createCatalogUnit)
	mux.HandleFunc("POST /api/catalog/methods", application.createCatalogMethod)
	mux.HandleFunc("POST /api/catalog/analytes", application.createCatalogAnalyte)
	mux.HandleFunc("POST /api/catalog/services", application.createAnalysisService)
	mux.HandleFunc("POST /api/catalog/profiles", application.createAnalysisProfile)
	mux.HandleFunc("POST /api/sample-reference", application.createSampleReferenceItem)
	mux.HandleFunc("POST /api/sample-reference/", application.updateSampleReferenceItem)
	mux.HandleFunc("DELETE /api/sample-reference/", application.deleteSampleReferenceItem)
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
	seedActor := cliActor("psc-operator-seed", lab.RoleAdmin)
	client, err := store.CreateClient("Clearline Synthetic Lab", "synthetic@example.test", seedActor)
	if err != nil {
		return err
	}
	if _, err := store.CreateSample(lab.CreateSampleInput{ClientID: client.ID, Project: "Synthetic Drinking Water Compliance", Matrix: "Water", Tests: []string{"pH", "Turbidity", "Lead"}}, seedActor); err != nil {
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

func mvpVerticalSlice(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("mvp vertical-slice", flag.ContinueOnError)
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
	summary, err := store.RunMVPVerticalSlice(lab.MVPVerticalSliceInput{}, cliActor("psc-mvp-operator", lab.RoleAdmin, lab.RoleLabManager))
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "mvp vertical-slice ok db=%s sample=%s worksheet=%s report_artifact=%s denied_controls=%d\n", *dbPath, summary.Sample.ID, summary.Worksheet.ID, summary.Report.Artifact.ID, len(summary.DeniedControls))
	return nil
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
	references := a.store.AllSampleReferenceItemsForScope(scope)
	return pageData{
		Scope:                       scope,
		Clients:                     a.store.ClientsForScope(scope),
		Sites:                       a.store.SitesForScope(scope),
		Contacts:                    a.store.ContactsForScope(scope),
		ContactRoles:                a.store.ContactRolesForScope(scope),
		Projects:                    a.store.ProjectsForScope(scope),
		ClientDefaults:              a.store.AllClientDefaultsForScope(scope),
		Samples:                     a.store.SamplesForScope(scope),
		Results:                     a.store.ResultsForScope(scope),
		Audit:                       audit,
		Departments:                 a.store.CatalogDepartmentsForScope(scope),
		Units:                       a.store.CatalogUnitsForScope(scope),
		Methods:                     a.store.CatalogMethodsForScope(scope),
		Analytes:                    a.store.CatalogAnalytesForScope(scope),
		Services:                    a.store.AnalysisServicesForScope(scope),
		Profiles:                    a.store.AnalysisProfilesForScope(scope),
		AnalysisRequestLines:        a.store.AnalysisRequestLinesForScope(scope),
		Worksheets:                  a.store.WorksheetsForScope(scope),
		SampleReference:             references,
		MatrixReferences:            sampleReferencesByKind(references, lab.SampleReferenceMatrix),
		ContainerReferences:         sampleReferencesByKind(references, lab.SampleReferenceContainer),
		PreservativeReferences:      sampleReferencesByKind(references, lab.SampleReferencePreservative),
		StorageLocationReferences:   sampleReferencesByKind(references, lab.SampleReferenceStorageLocation),
		ReceivedConditionReferences: sampleReferencesByKind(references, lab.SampleReferenceReceivedCondition),
	}
}

func sampleReferencesByKind(items []lab.SampleReferenceItem, kind lab.SampleReferenceKind) []lab.SampleReferenceItem {
	filtered := make([]lab.SampleReferenceItem, 0, len(items))
	for _, item := range items {
		if item.Kind == kind {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (a *app) demoReset(w http.ResponseWriter, r *http.Request) {
	if !a.demoResetEnabled {
		http.NotFound(w, r)
		return
	}
	summary, err := a.store.ResetAndSeedSyntheticDemo(a.fixturePath, demoResetActor(r))
	if err != nil {
		if errors.Is(err, lab.ErrAuthorizationDenied) {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, summary, http.StatusOK)
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

func (a *app) createSampleIntakeTemplate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tmpl, err := a.store.CreateSampleIntakeTemplateForScope(scopeFromRequest(r), lab.SampleIntakeTemplateInput{
		Name:                r.FormValue("name"),
		ClientID:            r.FormValue("client_id"),
		ProjectID:           r.FormValue("project_id"),
		Project:             r.FormValue("project"),
		Matrix:              r.FormValue("matrix"),
		MatrixReferenceID:   r.FormValue("matrix_reference_id"),
		ContainerID:         r.FormValue("container_id"),
		PreservativeID:      r.FormValue("preservative_id"),
		StorageLocationID:   r.FormValue("storage_location_id"),
		ReceivedConditionID: r.FormValue("received_condition_id"),
		Priority:            lab.SamplePriority(r.FormValue("priority")),
		AnalysisProfileIDs:  splitIDs(r.Form["analysis_profile_ids"]),
		AnalysisServiceIDs:  splitIDs(r.Form["analysis_service_ids"]),
		Tests:               splitTests(r.FormValue("tests")),
	}, actor(r))
	writeMutationResponse(w, r, tmpl, err)
}

func (a *app) createSamplesFromTemplate(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/sample-intake-templates/"), "/samples")
	if templateID == "" || strings.Contains(templateID, "/") || !strings.HasSuffix(r.URL.Path, "/samples") {
		http.NotFound(w, r)
		return
	}
	var rows []lab.SampleTemplateRowInput
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&rows); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		clientIDs := splitTests(r.FormValue("client_sample_ids"))
		labIDs := splitTests(r.FormValue("lab_sample_ids"))
		for i, clientID := range clientIDs {
			row := lab.SampleTemplateRowInput{ClientSampleID: clientID}
			if i < len(labIDs) {
				row.LabSampleID = labIDs[i]
			}
			rows = append(rows, row)
		}
	}
	samples, err := a.store.CreateSamplesFromTemplateForScope(scopeFromRequest(r), templateID, rows, actor(r))
	writeMutationResponse(w, r, samples, err)
}

func (a *app) createResult(w http.ResponseWriter, r *http.Request) {
	input, ok := resultInputFromRequest(w, r)
	if !ok {
		return
	}
	result, err := a.store.CreateResultForScope(scopeFromRequest(r), input, actor(r))
	writeMutationResponse(w, r, result, err)
}

func (a *app) resultAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/results/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(path, "/review") {
		resultID := strings.TrimSuffix(path, "/review")
		if resultID == "" || strings.Contains(resultID, "/") {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := a.store.ReviewResultForScope(scopeFromRequest(r), resultID, lab.ResultReviewInput{Decision: lab.ResultDecision(r.FormValue("decision")), Comments: r.FormValue("review_comments"), EnforceReviewerSeparation: parseBool(r.FormValue("enforce_reviewer_separation"))}, actor(r))
		writeMutationResponse(w, r, result, err)
		return
	}
	if strings.HasSuffix(path, "/reopen") {
		resultID := strings.TrimSuffix(path, "/reopen")
		if resultID == "" || strings.Contains(resultID, "/") {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := a.store.ReopenResultForScope(scopeFromRequest(r), resultID, r.FormValue("reason"), actor(r))
		writeMutationResponse(w, r, result, err)
		return
	}
	if strings.Contains(path, "/") {
		http.NotFound(w, r)
		return
	}
	input, ok := resultInputFromRequest(w, r)
	if !ok {
		return
	}
	result, err := a.store.UpdateResultForScope(scopeFromRequest(r), path, input, actor(r))
	writeMutationResponse(w, r, result, err)
}

func resultInputFromRequest(w http.ResponseWriter, r *http.Request) (lab.ResultInput, bool) {
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var input lab.ResultInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return lab.ResultInput{}, false
		}
		return input, true
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return lab.ResultInput{}, false
	}
	return lab.ResultInput{
		AnalysisRequestLineID: r.FormValue("analysis_request_line_id"),
		Value:                 parseFloat(r.FormValue("value")),
		RawValue:              r.FormValue("raw_value"),
		Unit:                  r.FormValue("unit"),
		Qualifier:             r.FormValue("qualifier"),
		MDL:                   parseFloat(r.FormValue("mdl")),
		RL:                    parseFloat(r.FormValue("rl")),
		LOQ:                   parseFloat(r.FormValue("loq")),
		Dilution:              parseFloatDefault(r.FormValue("dilution"), 1),
		Uncertainty:           parseFloat(r.FormValue("uncertainty")),
		Comments:              r.FormValue("comments"),
		AnalystID:             r.FormValue("analyst_id"),
		InstrumentID:          r.FormValue("instrument_id"),
	}, true
}

func parseFloat(raw string) float64 {
	value, _ := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return value
}

func parseFloatDefault(raw string, fallback float64) float64 {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	return parseFloat(raw)
}

func parseBool(raw string) bool {
	value, _ := strconv.ParseBool(strings.TrimSpace(raw))
	return value
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
	input := lab.CreateSampleInput{
		ClientID:            r.FormValue("client_id"),
		ProjectID:           r.FormValue("project_id"),
		Project:             r.FormValue("project"),
		ClientSampleID:      r.FormValue("client_sample_id"),
		LabSampleID:         r.FormValue("lab_sample_id"),
		Matrix:              r.FormValue("matrix"),
		MatrixReferenceID:   r.FormValue("matrix_reference_id"),
		ContainerID:         r.FormValue("container_id"),
		PreservativeID:      r.FormValue("preservative_id"),
		StorageLocationID:   r.FormValue("storage_location_id"),
		ReceivedConditionID: r.FormValue("received_condition_id"),
		SampledAt:           parseOptionalRequestTime(r.FormValue("sampled_at")),
		ReceivedAt:          parseOptionalRequestTime(r.FormValue("received_at")),
		Priority:            lab.SamplePriority(r.FormValue("priority")),
		Comments:            r.FormValue("comments"),
		AnalysisProfileIDs:  splitIDs(r.Form["analysis_profile_ids"]),
		AnalysisServiceIDs:  splitIDs(r.Form["analysis_service_ids"]),
		Tests:               splitTests(r.FormValue("tests")),
		Containers:          sampleContainerInputsFromRequest(r),
	}
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

func (a *app) sampleAction(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/transition") {
		a.transitionSample(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/custody-events") {
		a.recordCustodyEvent(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/coc-package") {
		a.generateCOCPackage(w, r)
		return
	}
	http.NotFound(w, r)
}

func (a *app) sampleLabelArtifact(w http.ResponseWriter, r *http.Request) {
	sampleID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/samples/"), "/label-artifact")
	if sampleID == "" || strings.Contains(sampleID, "/") || !strings.HasSuffix(r.URL.Path, "/label-artifact") {
		http.NotFound(w, r)
		return
	}
	artifact, err := a.store.GenerateSampleLabelArtifactForScope(scopeFromRequest(r), sampleID, actor(r))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, lab.ErrAuthorizationDenied) {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, artifact, http.StatusOK)
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

func (a *app) recordCustodyEvent(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/custody-events") {
		http.NotFound(w, r)
		return
	}
	sampleID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/samples/"), "/custody-events")
	if sampleID == "" || strings.Contains(sampleID, "/") {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	event, err := a.store.RecordCustodyEventForScope(scopeFromRequest(r), lab.CustodyEventInput{SampleID: sampleID, Type: lab.CustodyEventType(r.FormValue("custody_type")), Location: r.FormValue("custody_location"), Reason: r.FormValue("custody_reason"), OccurredAt: parseOptionalRequestTime(r.FormValue("custody_occurred_at"))}, actor(r))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, lab.ErrAuthorizationDenied) {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, event, http.StatusCreated)
		return
	}
	http.Redirect(w, r, scopedHome(scopeFromRequest(r)), http.StatusSeeOther)
}

func (a *app) generateCOCPackage(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/coc-package") {
		http.NotFound(w, r)
		return
	}
	sampleID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/samples/"), "/coc-package")
	if sampleID == "" || strings.Contains(sampleID, "/") {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pkg, err := a.store.GenerateCOCPackageForScope(scopeFromRequest(r), lab.COCPackageInput{SampleID: sampleID, PackageFormat: r.FormValue("package_format"), Attachments: cocPackageAttachmentsFromRequest(r)}, actor(r))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, lab.ErrAuthorizationDenied) {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, pkg, http.StatusCreated)
		return
	}
	http.Redirect(w, r, scopedHome(scopeFromRequest(r)), http.StatusSeeOther)
}

func cocPackageAttachmentsFromRequest(r *http.Request) []lab.ReportPackageAttachmentInput {
	names := r.Form["attachment_name"]
	mediaTypes := r.Form["attachment_media_type"]
	contents := r.Form["attachment_content_text"]
	sourceArtifactIDs := r.Form["attachment_source_artifact_id"]
	attachments := make([]lab.ReportPackageAttachmentInput, 0, len(names))
	for i, name := range names {
		mediaType := valueAt(mediaTypes, i)
		content := valueAt(contents, i)
		sourceArtifactID := valueAt(sourceArtifactIDs, i)
		attachments = append(attachments, lab.ReportPackageAttachmentInput{Name: name, MediaType: mediaType, Content: []byte(content), SourceArtifactID: sourceArtifactID})
	}
	return attachments
}

func valueAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func (a *app) createWorksheet(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	worksheet, err := a.store.CreateWorksheetForScope(scopeFromRequest(r), lab.CreateWorksheetInput{AnalysisRequestLineIDs: splitIDs(r.Form["analysis_request_line_ids"]), BatchID: r.FormValue("batch_id"), AnalystID: r.FormValue("analyst_id")}, actor(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, worksheet, http.StatusCreated)
		return
	}
	http.Redirect(w, r, scopedHome(scopeFromRequest(r)), http.StatusSeeOther)
}

func (a *app) routeWorksheetMutation(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/worksheets/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	worksheetID := parts[0]
	var err error
	switch {
	case len(parts) == 2 && parts[1] == "assign":
		err = a.store.AssignWorksheetAnalystForScope(scopeFromRequest(r), worksheetID, r.FormValue("analyst_id"), actor(r))
	case len(parts) == 2 && parts[1] == "transition":
		err = a.store.TransitionWorksheetForScope(scopeFromRequest(r), worksheetID, lab.WorksheetStatus(r.FormValue("status")), actor(r))
	case len(parts) == 4 && parts[1] == "lines" && parts[3] == "remove":
		err = a.store.RemoveWorksheetLineForScope(scopeFromRequest(r), worksheetID, parts[2], actor(r))
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
		return
	}
	http.Redirect(w, r, scopedHome(scopeFromRequest(r)), http.StatusSeeOther)
}

func (a *app) createCatalogDepartment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	department, err := a.store.CreateCatalogDepartmentForScope(scopeFromRequest(r), lab.CatalogDepartmentInput{Name: r.FormValue("name"), SortOrder: parseInt(r.FormValue("sort_order"))}, actor(r))
	writeCatalogResult(w, r, department, err)
}

func (a *app) createCatalogUnit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	unit, err := a.store.CreateCatalogUnitForScope(scopeFromRequest(r), lab.CatalogUnitInput{Name: r.FormValue("name"), Symbol: r.FormValue("symbol")}, actor(r))
	writeCatalogResult(w, r, unit, err)
}

func (a *app) createCatalogMethod(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	method, err := a.store.CreateCatalogMethodForScope(scopeFromRequest(r), lab.CatalogMethodInput{Name: r.FormValue("name"), Description: r.FormValue("description")}, actor(r))
	writeCatalogResult(w, r, method, err)
}

func (a *app) createCatalogAnalyte(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	analyte, err := a.store.CreateCatalogAnalyteForScope(scopeFromRequest(r), lab.CatalogAnalyteInput{Name: r.FormValue("name"), DefaultUnitID: r.FormValue("default_unit_id")}, actor(r))
	writeCatalogResult(w, r, analyte, err)
}

func (a *app) createAnalysisService(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	service, err := a.store.CreateAnalysisServiceForScope(scopeFromRequest(r), lab.AnalysisServiceInput{Name: r.FormValue("name"), DepartmentID: r.FormValue("department_id"), MethodID: r.FormValue("method_id"), AnalyteID: r.FormValue("analyte_id"), UnitID: r.FormValue("unit_id"), SortOrder: parseInt(r.FormValue("sort_order"))}, actor(r))
	writeCatalogResult(w, r, service, err)
}

func (a *app) createAnalysisProfile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	profile, err := a.store.CreateAnalysisProfileForScope(scopeFromRequest(r), lab.AnalysisProfileInput{Name: r.FormValue("name"), ServiceIDs: splitIDs(r.Form["service_ids"])}, actor(r))
	writeCatalogResult(w, r, profile, err)
}

func (a *app) createSampleReferenceItem(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	item, err := a.store.CreateSampleReferenceItemForScope(scopeFromRequest(r), sampleReferenceInputFromRequest(r), actor(r))
	writeCatalogResult(w, r, item, err)
}

func (a *app) updateSampleReferenceItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/sample-reference/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	item, err := a.store.UpdateSampleReferenceItemForScope(scopeFromRequest(r), id, sampleReferenceInputFromRequest(r), actor(r))
	writeCatalogResult(w, r, item, err)
}

func (a *app) deleteSampleReferenceItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/sample-reference/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	if err := a.store.DeleteSampleReferenceItemForScope(scopeFromRequest(r), id, actor(r)); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, lab.ErrAuthorizationDenied) {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func sampleReferenceInputFromRequest(r *http.Request) lab.SampleReferenceItemInput {
	active := true
	if raw := strings.TrimSpace(r.FormValue("active")); raw != "" {
		active = raw == "1" || strings.EqualFold(raw, "true") || strings.EqualFold(raw, "on") || strings.EqualFold(raw, "yes")
	}
	return lab.SampleReferenceItemInput{Kind: lab.SampleReferenceKind(r.FormValue("kind")), Name: r.FormValue("name"), Code: r.FormValue("code"), Description: r.FormValue("description"), SortOrder: parseInt(r.FormValue("sort_order")), Active: active}
}

func writeCatalogResult(w http.ResponseWriter, r *http.Request, value any, err error) {
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, lab.ErrAuthorizationDenied) {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, value, http.StatusCreated)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func parseInt(raw string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(raw))
	return value
}

func splitIDs(values []string) []string {
	ids := []string{}
	for _, value := range values {
		for _, id := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' }) {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				ids = append(ids, trimmed)
			}
		}
	}
	return ids
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

func sampleContainerInputsFromRequest(r *http.Request) []lab.SampleContainerInput {
	containerID := strings.TrimSpace(r.FormValue("container_id"))
	volume := strings.TrimSpace(r.FormValue("container_volume"))
	condition := strings.TrimSpace(r.FormValue("container_condition"))
	instructions := strings.TrimSpace(r.FormValue("aliquot_instructions"))
	aliquot := lab.SampleAliquotInput{
		DepartmentID: strings.TrimSpace(r.FormValue("aliquot_department_id")),
		MethodID:     strings.TrimSpace(r.FormValue("aliquot_method_id")),
		Volume:       strings.TrimSpace(r.FormValue("aliquot_volume")),
		Purpose:      strings.TrimSpace(r.FormValue("aliquot_purpose")),
	}
	if containerID == "" && volume == "" && condition == "" && instructions == "" && aliquot.DepartmentID == "" && aliquot.MethodID == "" && aliquot.Volume == "" && aliquot.Purpose == "" {
		return nil
	}
	input := lab.SampleContainerInput{
		ContainerReferenceID: containerID,
		PreservativeID:       r.FormValue("preservative_id"),
		ReceivedConditionID:  r.FormValue("received_condition_id"),
		Volume:               volume,
		Condition:            condition,
		AliquotInstructions:  instructions,
	}
	if aliquot.DepartmentID != "" || aliquot.MethodID != "" || aliquot.Volume != "" || aliquot.Purpose != "" {
		input.Aliquots = []lab.SampleAliquotInput{aliquot}
	}
	return []lab.SampleContainerInput{input}
}

func parseOptionalRequestTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func demoResetActor(r *http.Request) lab.ActorContext {
	requestID := requestID(r)
	return lab.MustActorContext(lab.ActorContextInput{
		UserID:            "local-demo-reset-admin",
		DisplayName:       "local-demo-reset-admin",
		AuthProvider:      "local-dev-demo-reset",
		RequestID:         requestID,
		CorrelationID:     requestID,
		TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleAdmin)}}},
		Roles:             []string{string(lab.RoleAdmin)},
	})
}

func actor(r *http.Request) lab.ActorContext {
	requestID := requestID(r)
	scope := scopeFromRequest(r)
	roles := []string{string(lab.RoleLabManager), string(lab.RoleAnalyst), string(lab.RoleReviewer), string(lab.RoleReportReleaser)}
	memberships := []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: roles}}
	if scope.TenantID != lab.DefaultTenantID {
		memberships = append(memberships, lab.TenantMembership{TenantID: scope.TenantID, Roles: roles})
	}
	return lab.MustActorContext(lab.ActorContextInput{
		UserID:            "lab-dev",
		DisplayName:       "lab-dev",
		AuthProvider:      "local-dev",
		RequestID:         requestID,
		CorrelationID:     requestID,
		TenantMemberships: memberships,
		Roles:             roles,
	})
}

func scopeFromRequest(r *http.Request) lab.Scope {
	scope := lab.Scope{TenantID: lab.DefaultTenantID, LabID: lab.DefaultLabID}
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

func requestID(r *http.Request) string {
	requestID := strings.TrimSpace(r.Header.Get("X-PSC-Request-ID"))
	if requestID == "" {
		requestID = "local-http-request"
	}
	return requestID
}

func cliActor(userID string, roles ...lab.Role) lab.ActorContext {
	roleStrings := make([]string, 0, len(roles))
	for _, role := range roles {
		roleStrings = append(roleStrings, string(role))
	}
	return lab.MustActorContext(lab.ActorContextInput{
		UserID:            userID,
		DisplayName:       userID,
		AuthProvider:      "local-cli",
		RequestID:         "local-cli-request",
		CorrelationID:     "local-cli-request",
		TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: roleStrings}},
		Roles:             roleStrings,
	})
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
