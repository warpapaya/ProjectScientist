package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
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
	sessions         map[string]authenticatedSession
	now              func() time.Time
}

type pageData struct {
	Scope                       lab.Scope
	CSRFToken                   string
	ActivePage                  string
	PageTitle                   string
	PageDescription             string
	DemoResetEnabled            bool
	Clients                     []lab.Client
	Sites                       []lab.Site
	Contacts                    []lab.Contact
	ContactRoles                []lab.ContactRole
	Projects                    []lab.Project
	ClientDefaults              []lab.ClientDefaults
	Samples                     []lab.Sample
	Results                     []lab.Result
	ReportReadiness             []lab.ReportReleaseReadiness
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
		if len(args) >= 3 {
			switch args[2] {
			case "vertical-slice":
				return mvpVerticalSlice(args[3:], stdout, stderr)
			case "verify-suite":
				return mvpVerifySuite(args[3:], stdout, stderr)
			}
		}
	case "customer-workflow":
		if len(args) >= 3 && args[2] == "smoke-matrix" {
			return customerWorkflowSmokeMatrix(args[3:], stdout, stderr)
		}
	case "smoke":
		if len(args) >= 3 {
			switch args[2] {
			case "performance":
				return smokePerformance(args[3:], stdout, stderr)
			case "prospect-trial":
				return smokeProspectTrial(args[3:], stdout, stderr)
			}
		}
		return smokeHTTP(args[2:], stdout, stderr)
	}
	return fmt.Errorf("unknown command; supported: serve, audit verify, db migrate, db status, seed, reset, backup, restore, mvp vertical-slice, mvp verify-suite, customer-workflow smoke-matrix, smoke, smoke performance")
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
		sessions:         configuredInternalSessions(),
		now:              time.Now,
	}
	log.Printf("Project Scientist listening on %s", addr)
	return http.ListenAndServe(addr, securityHeaders(application.routes()))
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.index)
	mux.HandleFunc("GET /login", a.loginPage)
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("POST /logout", a.logout)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/state", a.apiState)
	mux.HandleFunc("POST /api/demo/reset", a.demoReset)
	mux.HandleFunc("POST /api/clients", a.createClient)
	mux.HandleFunc("POST /api/sites", a.createSite)
	mux.HandleFunc("POST /api/contacts", a.createContact)
	mux.HandleFunc("POST /api/contact-roles", a.assignContactRole)
	mux.HandleFunc("POST /api/projects", a.createProject)
	mux.HandleFunc("POST /api/client-defaults", a.upsertClientDefaults)
	mux.HandleFunc("POST /api/sample-intake-templates", a.createSampleIntakeTemplate)
	mux.HandleFunc("POST /api/sample-intake-templates/", a.createSamplesFromTemplate)
	mux.HandleFunc("POST /api/results", a.createResult)
	mux.HandleFunc("POST /api/results/", a.resultAction)
	mux.HandleFunc("POST /api/samples", a.createSample)
	mux.HandleFunc("GET /api/samples/", a.sampleDownloadAction)
	mux.HandleFunc("GET /api/report-artifacts/", a.reportArtifactDownload)
	mux.HandleFunc("GET /api/coc-packages/", a.cocPackageDownload)
	mux.HandleFunc("POST /api/samples/", a.sampleAction)
	mux.HandleFunc("POST /api/worksheets", a.createWorksheet)
	mux.HandleFunc("POST /api/worksheets/", a.routeWorksheetMutation)
	mux.HandleFunc("POST /api/catalog/departments", a.createCatalogDepartment)
	mux.HandleFunc("POST /api/catalog/units", a.createCatalogUnit)
	mux.HandleFunc("POST /api/catalog/methods", a.createCatalogMethod)
	mux.HandleFunc("POST /api/catalog/analytes", a.createCatalogAnalyte)
	mux.HandleFunc("POST /api/catalog/services", a.createAnalysisService)
	mux.HandleFunc("POST /api/catalog/profiles", a.createAnalysisProfile)
	mux.HandleFunc("POST /api/sample-reference", a.createSampleReferenceItem)
	mux.HandleFunc("POST /api/sample-reference/", a.updateSampleReferenceItem)
	mux.HandleFunc("DELETE /api/sample-reference/", a.deleteSampleReferenceItem)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	return a.requireSessionBoundary(mux)
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

type mvpVerificationArtifact struct {
	Status           string   `json:"status"`
	Command          string   `json:"command"`
	DBPath           string   `json:"db_path"`
	SampleID         string   `json:"sample_id"`
	WorksheetID      string   `json:"worksheet_id"`
	ReportArtifactID string   `json:"report_artifact_id"`
	NegativeControls []string `json:"negative_controls"`
	AuditActions     []string `json:"audit_actions"`
}

func mvpVerifySuite(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("mvp verify-suite", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", defaultDBPath(), "SQLite database path")
	artifactsDir := fs.String("artifacts", filepath.Join("artifacts", "mvp-verification"), "artifact output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := removeSQLiteFiles(*dbPath); err != nil {
		return err
	}
	store, err := lab.OpenSQLiteStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	summary, err := store.RunMVPVerticalSlice(lab.MVPVerticalSliceInput{}, cliActor("psc-mvp-verify-suite", lab.RoleAdmin, lab.RoleLabManager))
	if err != nil {
		return err
	}
	auditEvents, err := store.AuditEventsForScope(summary.Scope, 0)
	if err != nil {
		return err
	}
	artifact := mvpVerificationArtifact{
		Status:           "pass",
		Command:          fmt.Sprintf("project-scientist mvp verify-suite --db %s --artifacts %s", *dbPath, *artifactsDir),
		DBPath:           *dbPath,
		SampleID:         summary.Sample.ID,
		WorksheetID:      summary.Worksheet.ID,
		ReportArtifactID: summary.Report.Artifact.ID,
		NegativeControls: append([]string(nil), summary.DeniedControls...),
		AuditActions:     uniqueAuditActions(auditEvents),
	}
	if len(artifact.NegativeControls) != 5 {
		return fmt.Errorf("mvp verify-suite expected 5 negative controls, got %d: %v", len(artifact.NegativeControls), artifact.NegativeControls)
	}
	if err := os.MkdirAll(*artifactsDir, 0o755); err != nil {
		return err
	}
	artifactPath := filepath.Join(*artifactsDir, "mvp-verification-suite.json")
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(artifactPath, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "mvp verify-suite ok db=%s sample=%s worksheet=%s report_artifact=%s negative_controls=%d artifact=%s\n", *dbPath, summary.Sample.ID, summary.Worksheet.ID, summary.Report.Artifact.ID, len(summary.DeniedControls), artifactPath)
	return nil
}

func uniqueAuditActions(events []lab.AuditEvent) []string {
	seen := map[string]bool{}
	actions := []string{}
	for _, event := range events {
		if event.Action == "" || seen[event.Action] {
			continue
		}
		seen[event.Action] = true
		actions = append(actions, event.Action)
	}
	return actions
}

func customerWorkflowSmokeMatrix(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("customer-workflow smoke-matrix", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", defaultDBPath(), "SQLite database path")
	fixturePath := fs.String("fixture", filepath.Join("fixtures", "golden_migration_dataset.json"), "synthetic golden migration fixture path")
	gapReportPath := fs.String("gap-report", filepath.Join("docs", "customer-workflow-gap-report.md"), "source customer workflow gap report path")
	outDir := fs.String("out", filepath.Join("artifacts", "customer-workflow-smoke"), "artifact output directory")
	commandOutput := fs.String("command-output", "not run; caller did not provide command output", "captured command output evidence to embed in each lane")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := lab.OpenSQLiteStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	matrix, err := store.GenerateCustomerWorkflowSmokeMatrix(lab.CustomerWorkflowSmokeInput{FixturePath: *fixturePath, GapReportPath: *gapReportPath, OutputDir: *outDir, CommandOutput: *commandOutput}, cliActor("psc-customer-workflow-smoke", lab.RoleAdmin))
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "customer workflow smoke-matrix ok lanes=%d green=%d yellow=%d red=%d matrix=%s\n", len(matrix.Lanes), matrix.StatusCounts[lab.SmokeStatusGreen], matrix.StatusCounts[lab.SmokeStatusYellow], matrix.StatusCounts[lab.SmokeStatusRed], matrix.MatrixArtifactPath)
	return nil
}

func smokeProspectTrial(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("smoke prospect-trial", flag.ContinueOnError)
	fs.SetOutput(stderr)
	baseURL := fs.String("base-url", getenv("DEV_BASE_URL", "http://127.0.0.1:8097"), "running local Project Scientist base URL")
	username := fs.String("username", getenv("PSC_INTERNAL_SESSION_USER", "lab-dev"), "local dev username")
	password := fs.String("password", configuredLoginPassword(), "local dev password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	base := strings.TrimRight(*baseURL, "/")
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 10 * time.Second, Jar: jar}
	if err := expectHTTP(&client, base+"/healthz", "ok"); err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	if err := expectHTTP(&client, base+"/login", "Sign in to the lab workspace"); err != nil {
		return fmt.Errorf("login page: %w", err)
	}
	form := url.Values{}
	form.Set("username", *username)
	form.Set("password", *password)
	loginReq, err := http.NewRequest(http.MethodPost, base+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginResp, err := client.Do(loginReq)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	loginBody, err := readHTTPBody(loginResp)
	if err != nil {
		return err
	}
	if loginResp.StatusCode < 200 || loginResp.StatusCode >= 300 {
		return fmt.Errorf("login returned %s: %s", loginResp.Status, strings.TrimSpace(loginBody))
	}
	for _, want := range []string{"Dashboard", `href="/samples"`, `aria-label="Primary navigation"`} {
		if !strings.Contains(loginBody, want) {
			return fmt.Errorf("post-login dashboard missing %q", want)
		}
	}
	csrf, err := csrfTokenFromJar(jar, base)
	if err != nil {
		return err
	}
	resetReq, err := http.NewRequest(http.MethodPost, base+"/api/demo/reset", nil)
	if err != nil {
		return err
	}
	resetReq.Header.Set("Accept", "application/json")
	resetReq.Header.Set(csrfHeaderName, csrf)
	resetResp, err := client.Do(resetReq)
	if err != nil {
		return fmt.Errorf("demo reset request: %w", err)
	}
	resetBody, err := readHTTPBody(resetResp)
	if err != nil {
		return err
	}
	if resetResp.StatusCode < 200 || resetResp.StatusCode >= 300 {
		return fmt.Errorf("demo reset returned %s: %s", resetResp.Status, strings.TrimSpace(resetBody))
	}
	for _, want := range []string{"Okefenokee Synthetic Water Authority", "S-000001"} {
		if !strings.Contains(resetBody, want) {
			return fmt.Errorf("demo reset response missing %q: %s", want, strings.TrimSpace(resetBody))
		}
	}
	checks := []struct {
		path string
		want []string
	}{
		{"/dashboard", []string{`href="/dashboard" aria-current="page"`, "Active samples", "Load a realistic lab workspace"}},
		{"/samples", []string{`href="/samples" aria-current="page"`, "S-000001", "Okefenokee Synthetic Water Authority", `id="workflow-board"`}},
		{"/results", []string{`href="/results" aria-current="page"`, "Result entry grid", "Review/release queue"}},
		{"/reports", []string{`href="/reports" aria-current="page"`, "Report release desk", "Preview COA"}},
	}
	for _, check := range checks {
		body, err := getHTTPBody(&client, base+check.path)
		if err != nil {
			return fmt.Errorf("%s: %w", check.path, err)
		}
		for _, want := range check.want {
			if !strings.Contains(body, want) {
				return fmt.Errorf("%s missing %q", check.path, want)
			}
		}
	}
	fmt.Fprintf(stdout, "prospect trial smoke ok base_url=%s routes=login,dashboard,samples,results,reports seeded_sample=S-000001\n", base)
	return nil
}

func readHTTPBody(res *http.Response) (string, error) {
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func getHTTPBody(client *http.Client, url string) (string, error) {
	res, err := client.Get(url)
	if err != nil {
		return "", err
	}
	body, err := readHTTPBody(res)
	if err != nil {
		return "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("returned %s: %s", res.Status, strings.TrimSpace(body))
	}
	return body, nil
}

func csrfTokenFromJar(jar http.CookieJar, base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	for _, cookie := range jar.Cookies(parsed) {
		if cookie.Name == sessionCookieName && strings.TrimSpace(cookie.Value) != "" {
			return deriveCSRFToken(cookie.Value), nil
		}
	}
	return "", fmt.Errorf("%s cookie was not set by login", sessionCookieName)
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
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	activePage, title, description, ok := productionPageForPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	scope, actor, ok := a.requireAuthenticatedRequest(w, r)
	if !ok {
		return
	}
	data := a.pageData(scope, 20, actor)
	data.ActivePage = activePage
	data.PageTitle = title
	data.PageDescription = description
	if err := a.tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func productionPageForPath(path string) (activePage string, title string, description string, ok bool) {
	switch path {
	case "/":
		return "all", "Lab operations cockpit", "Run samples from intake to release without losing the audit trail.", true
	case "/dashboard":
		return "dashboard", "Dashboard", "Operational overview for today’s lab work.", true
	case "/samples":
		return "samples", "Samples", "Receive samples, track custody, and move work through legal workflow transitions.", true
	case "/results":
		return "results", "Results", "Build worksheets, enter results, and run technical review without mixing setup screens into the workbench.", true
	case "/reports":
		return "reports", "Reports", "Review blockers, preview COAs, release reports, and download custody packages.", true
	case "/admin":
		return "admin", "Administration", "Maintain clients, contacts, catalog, and controlled vocabulary away from the daily operations flow.", true
	case "/audit":
		return "audit", "Audit trail", "Inspect immutable workflow events and evidence hashes.", true
	default:
		return "", "", "", false
	}
}

func (a *app) apiState(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := a.requireAuthenticatedRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, a.pageData(scope, 50, actor), http.StatusOK)
}

func (a *app) pageData(scope lab.Scope, auditLimit int, actor lab.ActorContext) pageData {
	audit, _ := a.store.AuditEventsForScopeAsActor(scope, auditLimit, actor)
	references := a.store.AllSampleReferenceItemsForScope(scope)
	return pageData{
		Scope:                       scope,
		CSRFToken:                   a.csrfTokenForRequest(scope, actor),
		DemoResetEnabled:            a.demoResetEnabled,
		Clients:                     a.store.ClientsForScope(scope),
		Sites:                       a.store.SitesForScope(scope),
		Contacts:                    a.store.ContactsForScope(scope),
		ContactRoles:                a.store.ContactRolesForScope(scope),
		Projects:                    a.store.ProjectsForScope(scope),
		ClientDefaults:              a.store.AllClientDefaultsForScope(scope),
		Samples:                     a.store.SamplesForScope(scope),
		Results:                     a.store.ResultsForScope(scope),
		ReportReadiness:             a.store.ReportReleaseReadinessForScopeAll(scope),
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
	_, actor, ok := a.requireAuthenticatedRequest(w, r)
	if !ok {
		return
	}
	summary, err := a.store.ResetAndSeedSyntheticDemo(a.fixturePath, actor)
	if err != nil {
		if errors.Is(err, lab.ErrAuthorizationDenied) {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, summary, http.StatusOK)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (a *app) createClient(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, _, ok := a.requireAuthenticatedRequest(w, r); !ok {
		return
	}
	client, err := a.store.CreateClientForScope(a.sessionScope(r), r.FormValue("name"), r.FormValue("email"), a.sessionActor(r))
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, client, http.StatusCreated)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *app) createSite(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	site, err := a.store.CreateSiteForScope(a.sessionScope(r), lab.SiteInput{ClientID: r.FormValue("client_id"), Name: r.FormValue("name"), Division: r.FormValue("division"), Address: r.FormValue("address")}, a.sessionActor(r))
	writeMutationResponse(w, r, site, err)
}

func (a *app) createContact(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	contact, err := a.store.CreateContactForScope(a.sessionScope(r), lab.ContactInput{ClientID: r.FormValue("client_id"), SiteID: r.FormValue("site_id"), Name: r.FormValue("name"), Email: r.FormValue("email"), Phone: r.FormValue("phone")}, a.sessionActor(r))
	writeMutationResponse(w, r, contact, err)
}

func (a *app) assignContactRole(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	role, err := a.store.AssignContactRoleForScope(a.sessionScope(r), lab.ContactRoleInput{ContactID: r.FormValue("contact_id"), Role: r.FormValue("role")}, a.sessionActor(r))
	writeMutationResponse(w, r, role, err)
}

func (a *app) createProject(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	project, err := a.store.CreateProjectForScope(a.sessionScope(r), lab.ProjectInput{ClientID: r.FormValue("client_id"), SiteID: r.FormValue("site_id"), Name: r.FormValue("name"), WorkOrder: r.FormValue("work_order"), DefaultMatrix: r.FormValue("default_matrix"), DefaultTests: splitTests(r.FormValue("default_tests"))}, a.sessionActor(r))
	writeMutationResponse(w, r, project, err)
}

func (a *app) upsertClientDefaults(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defaults, err := a.store.UpsertClientDefaultsForScope(a.sessionScope(r), lab.ClientDefaultsInput{ClientID: r.FormValue("client_id"), ReportTemplate: r.FormValue("report_template"), InvoiceEmail: r.FormValue("invoice_email"), DefaultMatrix: r.FormValue("default_matrix"), DefaultTests: splitTests(r.FormValue("default_tests"))}, a.sessionActor(r))
	writeMutationResponse(w, r, defaults, err)
}

func (a *app) createSampleIntakeTemplate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tmpl, err := a.store.CreateSampleIntakeTemplateForScope(a.sessionScope(r), lab.SampleIntakeTemplateInput{
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
	}, a.sessionActor(r))
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
	samples, err := a.store.CreateSamplesFromTemplateForScope(a.sessionScope(r), templateID, rows, a.sessionActor(r))
	writeMutationResponse(w, r, samples, err)
}

func (a *app) createResult(w http.ResponseWriter, r *http.Request) {
	input, ok := resultInputFromRequest(w, r)
	if !ok {
		return
	}
	result, err := a.store.CreateResultForScope(a.sessionScope(r), input, a.sessionActor(r))
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
		result, err := a.store.ReviewResultForScope(a.sessionScope(r), resultID, lab.ResultReviewInput{Decision: lab.ResultDecision(r.FormValue("decision")), Comments: r.FormValue("review_comments"), EnforceReviewerSeparation: parseBool(r.FormValue("enforce_reviewer_separation"))}, a.sessionActor(r))
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
		result, err := a.store.ReopenResultForScope(a.sessionScope(r), resultID, r.FormValue("reason"), a.sessionActor(r))
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
	result, err := a.store.UpdateResultForScope(a.sessionScope(r), path, input, a.sessionActor(r))
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
		writeMutationError(w, err)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, value, http.StatusCreated)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
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
	sample, err := a.store.CreateSampleForScope(a.sessionScope(r), input, a.sessionActor(r))
	if err != nil {
		writeMutationError(w, err)
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
	if strings.HasSuffix(r.URL.Path, "/report-preview") {
		a.previewReportArtifact(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/report-release") {
		a.releaseReportArtifact(w, r)
		return
	}
	http.NotFound(w, r)
}

func (a *app) sampleDownloadAction(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/label-artifact") {
		a.sampleLabelArtifact(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/report-preview") {
		a.previewReportArtifact(w, r)
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
	artifact, err := a.store.GenerateSampleLabelArtifactForScope(a.sessionScope(r), sampleID, a.sessionActor(r))
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
	if err := a.store.TransitionSampleForScope(a.sessionScope(r), sampleID, lab.SampleStatus(r.FormValue("status")), a.sessionActor(r)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
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
	event, err := a.store.RecordCustodyEventForScope(a.sessionScope(r), lab.CustodyEventInput{SampleID: sampleID, Type: lab.CustodyEventType(r.FormValue("custody_type")), Location: r.FormValue("custody_location"), Reason: r.FormValue("custody_reason"), OccurredAt: parseOptionalRequestTime(r.FormValue("custody_occurred_at"))}, a.sessionActor(r))
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
	http.Redirect(w, r, "/", http.StatusSeeOther)
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
	pkg, err := a.store.GenerateCOCPackageForScope(a.sessionScope(r), lab.COCPackageInput{SampleID: sampleID, PackageFormat: r.FormValue("package_format"), Attachments: cocPackageAttachmentsFromRequest(r)}, a.sessionActor(r))
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
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *app) previewReportArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/report-preview") {
		http.NotFound(w, r)
		return
	}
	sampleID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/samples/"), "/report-preview")
	if sampleID == "" || strings.Contains(sampleID, "/") {
		http.NotFound(w, r)
		return
	}
	scope, _, ok := a.requireAuthenticatedRequest(w, r)
	if !ok {
		return
	}
	artifact, err := a.store.PreviewCOAArtifactForScope(scope, lab.COAGenerationInput{SampleID: sampleID, Template: defaultCOATemplateFromRequest(r)})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", artifact.Format)
	w.Header().Set("X-Project-Scientist-Preview", "true")
	_, _ = w.Write(artifact.Content)
}

func (a *app) releaseReportArtifact(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/report-release") {
		http.NotFound(w, r)
		return
	}
	sampleID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/samples/"), "/report-release")
	if sampleID == "" || strings.Contains(sampleID, "/") {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	released, err := a.store.GenerateCOAReportArtifactForScope(a.sessionScope(r), lab.COAGenerationInput{SampleID: sampleID, Template: defaultCOATemplateFromRequest(r)}, a.sessionActor(r))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, lab.ErrAuthorizationDenied) {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, released, http.StatusCreated)
		return
	}
	http.Redirect(w, r, "/#report-release", http.StatusSeeOther)
}

func defaultCOATemplateFromRequest(r *http.Request) lab.COATemplate {
	_ = r.ParseForm()
	templateID := strings.TrimSpace(r.FormValue("template_id"))
	if templateID == "" {
		templateID = "coa-standard"
	}
	templateVersion := strings.TrimSpace(r.FormValue("template_version"))
	if templateVersion == "" {
		templateVersion = "2026.06"
	}
	labName := strings.TrimSpace(r.FormValue("lab_name"))
	if labName == "" {
		labName = "Clearline Demo Lab"
	}
	clientName := strings.TrimSpace(r.FormValue("client_name"))
	if clientName == "" {
		clientName = "Synthetic Client"
	}
	return lab.COATemplate{ID: templateID, Version: templateVersion, Style: lab.COAStyleCENLA, LabName: labName, ClientName: clientName}
}

func (a *app) reportArtifactDownload(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/report-artifacts/"), "/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	scope, _, ok := a.requireAuthenticatedRequest(w, r)
	if !ok {
		return
	}
	artifact, ok := a.store.ReportArtifactForScope(scope, id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	contentType := artifact.Format
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifact.ID+".txt"))
	_, _ = w.Write(artifact.Content)
}

func (a *app) cocPackageDownload(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/coc-packages/"), "/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	scope, _, ok := a.requireAuthenticatedRequest(w, r)
	if !ok {
		return
	}
	pkg, ok := a.store.COCPackageForScope(scope, id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	contentType := pkg.PackageFormat
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", pkg.ID+".json"))
	_, _ = w.Write(pkg.Content)
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
	worksheet, err := a.store.CreateWorksheetForScope(a.sessionScope(r), lab.CreateWorksheetInput{AnalysisRequestLineIDs: splitIDs(r.Form["analysis_request_line_ids"]), BatchID: r.FormValue("batch_id"), AnalystID: r.FormValue("analyst_id")}, a.sessionActor(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, worksheet, http.StatusCreated)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
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
		err = a.store.AssignWorksheetAnalystForScope(a.sessionScope(r), worksheetID, r.FormValue("analyst_id"), a.sessionActor(r))
	case len(parts) == 2 && parts[1] == "transition":
		err = a.store.TransitionWorksheetForScope(a.sessionScope(r), worksheetID, lab.WorksheetStatus(r.FormValue("status")), a.sessionActor(r))
	case len(parts) == 4 && parts[1] == "lines" && parts[3] == "remove":
		err = a.store.RemoveWorksheetLineForScope(a.sessionScope(r), worksheetID, parts[2], a.sessionActor(r))
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
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *app) createCatalogDepartment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	department, err := a.store.CreateCatalogDepartmentForScope(a.sessionScope(r), lab.CatalogDepartmentInput{Name: r.FormValue("name"), SortOrder: parseInt(r.FormValue("sort_order"))}, a.sessionActor(r))
	writeCatalogResult(w, r, department, err)
}

func (a *app) createCatalogUnit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	unit, err := a.store.CreateCatalogUnitForScope(a.sessionScope(r), lab.CatalogUnitInput{Name: r.FormValue("name"), Symbol: r.FormValue("symbol")}, a.sessionActor(r))
	writeCatalogResult(w, r, unit, err)
}

func (a *app) createCatalogMethod(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	method, err := a.store.CreateCatalogMethodForScope(a.sessionScope(r), lab.CatalogMethodInput{Name: r.FormValue("name"), Description: r.FormValue("description")}, a.sessionActor(r))
	writeCatalogResult(w, r, method, err)
}

func (a *app) createCatalogAnalyte(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	analyte, err := a.store.CreateCatalogAnalyteForScope(a.sessionScope(r), lab.CatalogAnalyteInput{Name: r.FormValue("name"), DefaultUnitID: r.FormValue("default_unit_id")}, a.sessionActor(r))
	writeCatalogResult(w, r, analyte, err)
}

func (a *app) createAnalysisService(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	service, err := a.store.CreateAnalysisServiceForScope(a.sessionScope(r), lab.AnalysisServiceInput{Name: r.FormValue("name"), DepartmentID: r.FormValue("department_id"), MethodID: r.FormValue("method_id"), AnalyteID: r.FormValue("analyte_id"), UnitID: r.FormValue("unit_id"), SortOrder: parseInt(r.FormValue("sort_order"))}, a.sessionActor(r))
	writeCatalogResult(w, r, service, err)
}

func (a *app) createAnalysisProfile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	profile, err := a.store.CreateAnalysisProfileForScope(a.sessionScope(r), lab.AnalysisProfileInput{Name: r.FormValue("name"), ServiceIDs: splitIDs(r.Form["service_ids"])}, a.sessionActor(r))
	writeCatalogResult(w, r, profile, err)
}

func (a *app) createSampleReferenceItem(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	item, err := a.store.CreateSampleReferenceItemForScope(a.sessionScope(r), sampleReferenceInputFromRequest(r), a.sessionActor(r))
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
	item, err := a.store.UpdateSampleReferenceItemForScope(a.sessionScope(r), id, sampleReferenceInputFromRequest(r), a.sessionActor(r))
	writeCatalogResult(w, r, item, err)
}

func (a *app) deleteSampleReferenceItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/sample-reference/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	if err := a.store.DeleteSampleReferenceItemForScope(a.sessionScope(r), id, a.sessionActor(r)); err != nil {
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

func (a *app) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, err := a.currentSession(r); err == nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	a.renderLogin(w, http.StatusOK, "")
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	token, ok := a.sessionTokenForLogin(r.FormValue("username"), r.FormValue("password"))
	if !ok {
		a.renderLogin(w, http.StatusUnauthorized, "The username or password was not recognized.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *app) renderLogin(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	data := struct{ Message string }{Message: message}
	if err := loginTemplate.Execute(w, data); err != nil {
		log.Printf("render login: %v", err)
	}
}

func (a *app) sessionTokenForLogin(username, password string) (string, bool) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	expectedPassword := configuredLoginPassword()
	if a == nil || len(a.sessions) == 0 || username == "" || password == "" || expectedPassword == "" {
		return "", false
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(expectedPassword)) != 1 {
		return "", false
	}
	now := time.Now
	if a.now != nil {
		now = a.now
	}
	for token, session := range a.sessions {
		if session.Actor.UserID != username {
			continue
		}
		if !session.ExpiresAt.IsZero() && !now().Before(session.ExpiresAt) {
			continue
		}
		return token, true
	}
	return "", false
}

func configuredLoginPassword() string {
	return strings.TrimSpace(getenv("PSC_INTERNAL_SESSION_PASSWORD", "project-scientist-dev"))
}

var loginTemplate = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Sign in • Project Scientist</title>
  <link rel="stylesheet" href="/static/app.css">
</head>
<body class="login-body">
  <main class="login-shell">
    <section class="login-card-wrap">
      <div class="login-brand">
        <p class="eyebrow">Project Scientist</p>
        <h1>Sign in to the lab workspace</h1>
        <p>Use the local dev account to evaluate the v0.1 SaaS workflow without manually injecting cookies.</p>
      </div>
      <form class="login-card" method="post" action="/login">
        <h2>Welcome back</h2>
        {{if .Message}}<p class="login-error" role="alert">{{.Message}}</p>{{end}}
        <label><span>Username</span><input name="username" autocomplete="username" placeholder="lab-dev" required autofocus></label>
        <label><span>Password</span><input name="password" type="password" autocomplete="current-password" required></label>
        <button>Sign in</button>
        <p class="login-hint">Local default: <code>lab-dev</code> / <code>project-scientist-dev</code>. Override with <code>PSC_INTERNAL_SESSION_USER</code> and <code>PSC_INTERNAL_SESSION_PASSWORD</code>.</p>
      </form>
    </section>
  </main>
</body>
</html>`))

const (
	sessionCookieName   = "psc_internal_session"
	csrfHeaderName      = "X-PSC-CSRF-Token"
	csrfFormFieldName   = "csrf_token"
	csrfTokenDeriveSalt = "project-scientist-csrf-v1:"
)

type authenticatedSession struct {
	Actor     lab.ActorContext
	Scope     lab.Scope
	ExpiresAt time.Time
	CSRFToken string
}

func configuredInternalSessions() map[string]authenticatedSession {
	token := strings.TrimSpace(os.Getenv("PSC_INTERNAL_SESSION_TOKEN"))
	if token == "" {
		return map[string]authenticatedSession{}
	}
	tenantID := strings.TrimSpace(getenv("PSC_INTERNAL_SESSION_TENANT_ID", lab.DefaultTenantID))
	labID := strings.TrimSpace(getenv("PSC_INTERNAL_SESSION_LAB_ID", lab.DefaultLabID))
	userID := strings.TrimSpace(getenv("PSC_INTERNAL_SESSION_USER", "lab-dev"))
	roles := []string{string(lab.RoleAdmin), string(lab.RoleLabManager), string(lab.RoleAnalyst), string(lab.RoleReviewer), string(lab.RoleReportReleaser)}
	ttl := 12 * time.Hour
	if rawTTL := strings.TrimSpace(os.Getenv("PSC_INTERNAL_SESSION_TTL")); rawTTL != "" {
		if parsed, err := time.ParseDuration(rawTTL); err == nil {
			ttl = parsed
		}
	}
	csrfToken := strings.TrimSpace(os.Getenv("PSC_INTERNAL_CSRF_TOKEN"))
	if csrfToken == "" {
		csrfToken = deriveCSRFToken(token)
	}
	return map[string]authenticatedSession{
		token: {
			Actor: lab.MustActorContext(lab.ActorContextInput{
				UserID:            userID,
				DisplayName:       userID,
				AuthProvider:      "internal-session",
				TenantMemberships: []lab.TenantMembership{{TenantID: tenantID, Roles: roles}},
				Roles:             roles,
				RequestID:         "internal-session-bootstrap",
				CorrelationID:     "internal-session-bootstrap",
			}),
			Scope:     lab.Scope{TenantID: tenantID, LabID: labID},
			ExpiresAt: time.Now().Add(ttl),
			CSRFToken: csrfToken,
		},
	}
}

func (a *app) requireSessionBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/login" || r.URL.Path == "/logout" || strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		if _, _, ok := a.requireAuthenticatedRequest(w, r); !ok {
			return
		}
		session, err := a.currentSession(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if isCSRFProtectedMethod(r.Method) && !validCSRFToken(r, session.CSRFToken) {
			http.Error(w, "csrf token is required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) requireAuthenticatedRequest(w http.ResponseWriter, r *http.Request) (lab.Scope, lab.ActorContext, bool) {
	session, err := a.currentSession(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return lab.Scope{}, lab.ActorContext{}, false
	}
	if requested, ok := requestedScope(r); ok && (requested.TenantID != session.Scope.TenantID || requested.LabID != session.Scope.LabID) {
		http.Error(w, "request scope is not bound to authenticated session", http.StatusForbidden)
		return lab.Scope{}, lab.ActorContext{}, false
	}
	return session.Scope, a.sessionActor(r), true
}

func (a *app) currentSession(r *http.Request) (authenticatedSession, error) {
	if a == nil || len(a.sessions) == 0 {
		return authenticatedSession{}, errors.New("authenticated session is required")
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return authenticatedSession{}, errors.New("authenticated session is required")
	}
	session, ok := a.sessions[strings.TrimSpace(cookie.Value)]
	if !ok {
		return authenticatedSession{}, errors.New("authenticated session is invalid")
	}
	now := time.Now
	if a.now != nil {
		now = a.now
	}
	if !session.ExpiresAt.IsZero() && !now().Before(session.ExpiresAt) {
		return authenticatedSession{}, errors.New("authenticated session is expired")
	}
	if strings.TrimSpace(session.Scope.TenantID) == "" || strings.TrimSpace(session.Scope.LabID) == "" {
		return authenticatedSession{}, errors.New("authenticated session scope is invalid")
	}
	return session, nil
}

func (a *app) csrfTokenForRequest(scope lab.Scope, actor lab.ActorContext) string {
	if a == nil {
		return ""
	}
	for _, session := range a.sessions {
		if session.Actor.UserID == actor.UserID && session.Scope.TenantID == scope.TenantID && session.Scope.LabID == scope.LabID && session.CSRFToken != "" {
			return session.CSRFToken
		}
	}
	return ""
}

func isCSRFProtectedMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func validCSRFToken(r *http.Request, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return false
	}
	supplied := strings.TrimSpace(r.Header.Get(csrfHeaderName))
	if supplied == "" {
		supplied = strings.TrimSpace(r.FormValue(csrfFormFieldName))
	}
	return supplied != "" && subtle.ConstantTimeCompare([]byte(supplied), []byte(expected)) == 1
}

func deriveCSRFToken(sessionToken string) string {
	sum := sha256.Sum256([]byte(csrfTokenDeriveSalt + strings.TrimSpace(sessionToken)))
	return hex.EncodeToString(sum[:])
}

func requestedScope(r *http.Request) (lab.Scope, bool) {
	scope := lab.Scope{}
	selected := false
	if tenantID := strings.TrimSpace(r.Header.Get("X-PSC-Tenant-ID")); tenantID != "" {
		scope.TenantID = tenantID
		selected = true
	} else if tenantID := strings.TrimSpace(r.FormValue("tenant_id")); tenantID != "" {
		scope.TenantID = tenantID
		selected = true
	} else if tenantID := strings.TrimSpace(r.URL.Query().Get("tenant_id")); tenantID != "" {
		scope.TenantID = tenantID
		selected = true
	}
	if labID := strings.TrimSpace(r.Header.Get("X-PSC-Lab-ID")); labID != "" {
		scope.LabID = labID
		selected = true
	} else if labID := strings.TrimSpace(r.FormValue("lab_id")); labID != "" {
		scope.LabID = labID
		selected = true
	} else if labID := strings.TrimSpace(r.URL.Query().Get("lab_id")); labID != "" {
		scope.LabID = labID
		selected = true
	}
	if scope.TenantID == "" {
		scope.TenantID = lab.DefaultTenantID
	}
	if scope.LabID == "" {
		scope.LabID = lab.DefaultLabID
	}
	return scope, selected
}

func (a *app) sessionScope(r *http.Request) lab.Scope {
	if session, err := a.currentSession(r); err == nil {
		return session.Scope
	}
	return lab.Scope{TenantID: "unauthenticated-request", LabID: lab.DefaultLabID}
}

func (a *app) sessionActor(r *http.Request) lab.ActorContext {
	if session, err := a.currentSession(r); err == nil {
		actor := session.Actor
		actor.RequestID = requestID(r)
		actor.CorrelationID = requestID(r)
		return actor
	}
	return lab.MustActorContext(lab.ActorContextInput{
		UserID:        "unauthenticated-http-request",
		DisplayName:   "unauthenticated-http-request",
		AuthProvider:  "none",
		RequestID:     requestID(r),
		CorrelationID: requestID(r),
	})
}

func actor(r *http.Request) lab.ActorContext {
	requestID := requestID(r)
	roles := []string{string(lab.RoleLabManager), string(lab.RoleAnalyst), string(lab.RoleReviewer), string(lab.RoleReportReleaser)}
	memberships := []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: roles}}
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

func writeMutationError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, lab.ErrAuthorizationDenied) {
		status = http.StatusForbidden
	}
	http.Error(w, err.Error(), status)
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
	if scope.LabID != lab.DefaultLabID {
		return lab.Scope{TenantID: "unauthorized-request-scope", LabID: scope.LabID}
	}
	return scope
}

func (a *app) httpActorCanReadScope(r *http.Request, scope lab.Scope) bool {
	readActor := a.sessionActor(r)
	for _, membership := range readActor.TenantMemberships {
		if membership.TenantID == scope.TenantID && strings.TrimSpace(scope.LabID) == lab.DefaultLabID {
			return true
		}
	}
	return false
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
