package lab

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type SampleStatus string

const (
	StatusReceived   SampleStatus = "received"
	StatusInPrep     SampleStatus = "in_prep"
	StatusInAnalysis SampleStatus = "in_analysis"
	StatusInReview   SampleStatus = "in_review"
	StatusReleased   SampleStatus = "released"
)

const (
	DefaultTenantID = "lab-test"
	DefaultLabID    = "default-lab"
)

type Scope struct {
	TenantID string `json:"tenant_id"`
	LabID    string `json:"lab_id"`
}

func defaultScope() Scope { return Scope{TenantID: DefaultTenantID, LabID: DefaultLabID} }

var DefaultScope = defaultScope()

func normalizeScope(scope Scope) (Scope, error) {
	scope.TenantID = strings.TrimSpace(scope.TenantID)
	scope.LabID = strings.TrimSpace(scope.LabID)
	if scope.TenantID == "" {
		return Scope{}, errors.New("tenant id is required")
	}
	if scope.LabID == "" {
		return Scope{}, errors.New("lab id is required")
	}
	return scope, nil
}

type Client struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	LabID     string    `json:"lab_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type Analysis struct {
	ID                     string `json:"id"`
	TenantID               string `json:"tenant_id,omitempty"`
	LabID                  string `json:"lab_id,omitempty"`
	Name                   string `json:"name"`
	ServiceID              string `json:"service_id,omitempty"`
	ProfileID              string `json:"profile_id,omitempty"`
	Method                 string `json:"method,omitempty"`
	Result                 string `json:"result,omitempty"`
	Units                  string `json:"units,omitempty"`
	CatalogSnapshotID      string `json:"catalog_snapshot_id,omitempty"`
	CatalogSnapshotVersion int    `json:"catalog_snapshot_version,omitempty"`
}

type SamplePriority string

const (
	PriorityRoutine SamplePriority = "routine"
	PriorityRush    SamplePriority = "rush"
)

type Sample struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	LabID               string         `json:"lab_id"`
	ClientID            string         `json:"client_id"`
	ProjectID           string         `json:"project_id,omitempty"`
	Project             string         `json:"project"`
	ClientSampleID      string         `json:"client_sample_id,omitempty"`
	LabSampleID         string         `json:"lab_sample_id,omitempty"`
	Matrix              string         `json:"matrix"`
	MatrixReferenceID   string         `json:"matrix_reference_id,omitempty"`
	ContainerID         string         `json:"container_id,omitempty"`
	PreservativeID      string         `json:"preservative_id,omitempty"`
	StorageLocationID   string         `json:"storage_location_id,omitempty"`
	ReceivedConditionID string         `json:"received_condition_id,omitempty"`
	SampledAt           time.Time      `json:"sampled_at,omitempty"`
	ReceivedAt          time.Time      `json:"received_at,omitempty"`
	Priority            SamplePriority `json:"priority"`
	Comments            string         `json:"comments,omitempty"`
	Status              SampleStatus   `json:"status"`
	Analyses            []Analysis     `json:"analyses"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

type CreateSampleInput struct {
	ClientID            string         `json:"client_id"`
	ProjectID           string         `json:"project_id"`
	Project             string         `json:"project"`
	ClientSampleID      string         `json:"client_sample_id"`
	LabSampleID         string         `json:"lab_sample_id"`
	Matrix              string         `json:"matrix"`
	MatrixReferenceID   string         `json:"matrix_reference_id"`
	ContainerID         string         `json:"container_id"`
	PreservativeID      string         `json:"preservative_id"`
	StorageLocationID   string         `json:"storage_location_id"`
	ReceivedConditionID string         `json:"received_condition_id"`
	SampledAt           time.Time      `json:"sampled_at"`
	ReceivedAt          time.Time      `json:"received_at"`
	Priority            SamplePriority `json:"priority"`
	Comments            string         `json:"comments"`
	AnalysisProfileIDs  []string       `json:"analysis_profile_ids"`
	AnalysisServiceIDs  []string       `json:"analysis_service_ids"`
	Tests               []string       `json:"tests"`
}

type AuditOutcome string

const (
	AuditOutcomeAllowed AuditOutcome = "allowed"
	AuditOutcomeDenied  AuditOutcome = "denied"
	AuditOutcomeFailed  AuditOutcome = "failed"
	AuditOutcomeSystem  AuditOutcome = "system"
)

type AuditResource struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

type AuditEvent struct {
	EventID       string         `json:"event_id"`
	TenantID      string         `json:"tenant_id"`
	LabID         string         `json:"lab_id"`
	Timestamp     time.Time      `json:"timestamp"`
	Actor         string         `json:"actor"` // compatibility: stable actor user id
	ActorContext  ActorContext   `json:"actor_context"`
	Resource      AuditResource  `json:"resource"`
	Action        string         `json:"action"`
	Outcome       AuditOutcome   `json:"outcome"`
	Reason        string         `json:"reason,omitempty"`
	CorrelationID string         `json:"correlation_id"`
	Sequence      int64          `json:"sequence"`
	Details       map[string]any `json:"details,omitempty"`
	PreviousHash  string         `json:"previous_hash"`
	Hash          string         `json:"hash"`
}

type AuditCheckpoint struct {
	Name      string    `json:"name"`
	Sequence  int64     `json:"sequence"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	mu sync.Mutex
	db *sql.DB
}

type StoreRepository interface {
	CreateClient(name, email string, actor ActorContext) (Client, error)
	CreateSample(input CreateSampleInput, actor ActorContext) (Sample, error)
	TransitionSample(sampleID string, next SampleStatus, actor ActorContext) error
	GetSample(id string) (Sample, bool)
	Clients() []Client
	Samples() []Sample
	AuditEvents(limit int) ([]AuditEvent, error)
	Close() error
}

const sqliteSchemaVersion = 3

var sqliteMigrations = []string{
	`PRAGMA journal_mode = WAL;`,
	`PRAGMA foreign_keys = ON;`,
	`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);`,
	`CREATE TABLE IF NOT EXISTS store_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);`,
	`CREATE TABLE IF NOT EXISTS clients (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		email TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS samples (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		client_id TEXT NOT NULL REFERENCES clients(id),
		project_id TEXT NOT NULL DEFAULT '',
		project TEXT NOT NULL CHECK (length(trim(project)) > 0),
		client_sample_id TEXT NOT NULL DEFAULT '',
		lab_sample_id TEXT NOT NULL DEFAULT '',
		matrix TEXT NOT NULL,
		matrix_reference_id TEXT NOT NULL DEFAULT '',
		container_id TEXT NOT NULL DEFAULT '',
		preservative_id TEXT NOT NULL DEFAULT '',
		storage_location_id TEXT NOT NULL DEFAULT '',
		received_condition_id TEXT NOT NULL DEFAULT '',
		sampled_at TEXT NOT NULL DEFAULT '',
		received_at TEXT NOT NULL DEFAULT '',
		priority TEXT NOT NULL DEFAULT 'routine',
		comments TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		analyses_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS audit_events (
		event_id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		timestamp TEXT NOT NULL,
		actor TEXT NOT NULL,
		actor_json TEXT NOT NULL,
		resource_json TEXT NOT NULL,
		action TEXT NOT NULL CHECK (length(trim(action)) > 0),
		outcome TEXT NOT NULL CHECK (outcome IN ('allowed','denied','failed','system')),
		reason TEXT NOT NULL DEFAULT '',
		correlation_id TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		details_json TEXT NOT NULL,
		previous_hash TEXT NOT NULL,
		hash TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS audit_checkpoints (
		name TEXT PRIMARY KEY,
		sequence INTEGER NOT NULL,
		hash TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS sites (
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		id TEXT PRIMARY KEY,
		client_id TEXT NOT NULL REFERENCES clients(id),
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		division TEXT NOT NULL DEFAULT '',
		address TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS contacts (
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		id TEXT PRIMARY KEY,
		client_id TEXT NOT NULL REFERENCES clients(id),
		site_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		email TEXT NOT NULL DEFAULT '',
		phone TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS contact_roles (
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		id TEXT PRIMARY KEY,
		contact_id TEXT NOT NULL REFERENCES contacts(id),
		role TEXT NOT NULL CHECK (length(trim(role)) > 0),
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS projects (
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		id TEXT PRIMARY KEY,
		client_id TEXT NOT NULL REFERENCES clients(id),
		site_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		work_order TEXT NOT NULL DEFAULT '',
		default_matrix TEXT NOT NULL DEFAULT '',
		default_tests_json TEXT NOT NULL DEFAULT '[]',
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS client_defaults (
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		client_id TEXT NOT NULL REFERENCES clients(id),
		report_template TEXT NOT NULL DEFAULT '',
		invoice_email TEXT NOT NULL DEFAULT '',
		default_matrix TEXT NOT NULL DEFAULT '',
		default_tests_json TEXT NOT NULL DEFAULT '[]',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (tenant_id, lab_id, client_id)
	);`,
	`CREATE TABLE IF NOT EXISTS catalog_departments (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS catalog_units (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		symbol TEXT NOT NULL CHECK (length(trim(symbol)) > 0),
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS catalog_methods (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		description TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS catalog_analytes (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		default_unit_id TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS analysis_services (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		department_id TEXT NOT NULL,
		method_id TEXT NOT NULL DEFAULT '',
		analyte_id TEXT NOT NULL DEFAULT '',
		unit_id TEXT NOT NULL DEFAULT '',
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS analysis_profiles (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS analysis_profile_services (
		profile_id TEXT NOT NULL,
		service_id TEXT NOT NULL,
		sort_order INTEGER NOT NULL,
		PRIMARY KEY(profile_id, service_id)
	);`,
	`CREATE TABLE IF NOT EXISTS sample_reference_items (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		kind TEXT NOT NULL CHECK (kind IN ('matrix','container','preservative','storage_location','received_condition')),
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		code TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		sort_order INTEGER NOT NULL DEFAULT 0,
		active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(tenant_id, lab_id, kind, code)
	);`,
	`CREATE TABLE IF NOT EXISTS catalog_snapshots (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		version INTEGER NOT NULL,
		content_hash TEXT NOT NULL,
		data_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		UNIQUE(tenant_id, lab_id, version)
	);`,
	`CREATE TABLE IF NOT EXISTS sample_intake_templates (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		lab_id TEXT NOT NULL,
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		client_id TEXT NOT NULL REFERENCES clients(id),
		project_id TEXT NOT NULL DEFAULT '',
		project TEXT NOT NULL DEFAULT '',
		matrix TEXT NOT NULL DEFAULT '',
		matrix_reference_id TEXT NOT NULL DEFAULT '',
		container_id TEXT NOT NULL DEFAULT '',
		preservative_id TEXT NOT NULL DEFAULT '',
		storage_location_id TEXT NOT NULL DEFAULT '',
		received_condition_id TEXT NOT NULL DEFAULT '',
		priority TEXT NOT NULL DEFAULT 'routine',
		analysis_profile_ids_json TEXT NOT NULL DEFAULT '[]',
		analysis_service_ids_json TEXT NOT NULL DEFAULT '[]',
		tests_json TEXT NOT NULL DEFAULT '[]',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(tenant_id, lab_id, name)
	);`,
	`INSERT OR IGNORE INTO store_meta(key, value) VALUES ('next_client', '1'), ('next_sample', '1'), ('next_site', '1'), ('next_contact', '1'), ('next_contact_role', '1'), ('next_project', '1'), ('next_audit', '1'), ('next_catalog_department', '1'), ('next_catalog_unit', '1'), ('next_catalog_method', '1'), ('next_catalog_analyte', '1'), ('next_analysis_service', '1'), ('next_analysis_profile', '1'), ('next_sample_reference', '1'), ('next_catalog_snapshot', '1'), ('next_sample_intake_template', '1'), ('last_hash', '');`,
	`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (6, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));`,
}

var sqlitePostMigrationIndexes = []string{
	`CREATE INDEX IF NOT EXISTS idx_clients_scope ON clients(tenant_id, lab_id);`,
	`CREATE INDEX IF NOT EXISTS idx_samples_scope ON samples(tenant_id, lab_id);`,
	`CREATE INDEX IF NOT EXISTS idx_samples_client_id ON samples(client_id);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_samples_scope_client_sample_id ON samples(tenant_id, lab_id, client_id, client_sample_id) WHERE client_sample_id != '';`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_samples_scope_lab_sample_id ON samples(tenant_id, lab_id, lab_sample_id) WHERE lab_sample_id != '';`,
	`CREATE INDEX IF NOT EXISTS idx_audit_events_sequence ON audit_events(sequence);`,
	`CREATE INDEX IF NOT EXISTS idx_audit_events_scope ON audit_events(tenant_id, lab_id, sequence);`,
	`CREATE INDEX IF NOT EXISTS idx_sites_scope_client_id ON sites(tenant_id, lab_id, client_id);`,
	`CREATE INDEX IF NOT EXISTS idx_contacts_scope_client_id ON contacts(tenant_id, lab_id, client_id);`,
	`CREATE INDEX IF NOT EXISTS idx_contact_roles_scope_contact_id ON contact_roles(tenant_id, lab_id, contact_id);`,
	`CREATE INDEX IF NOT EXISTS idx_projects_scope_client_id ON projects(tenant_id, lab_id, client_id);`,
	`CREATE INDEX IF NOT EXISTS idx_catalog_departments_scope_order ON catalog_departments(tenant_id, lab_id, sort_order, name);`,
	`CREATE INDEX IF NOT EXISTS idx_catalog_units_scope ON catalog_units(tenant_id, lab_id, symbol);`,
	`CREATE INDEX IF NOT EXISTS idx_catalog_methods_scope ON catalog_methods(tenant_id, lab_id, name);`,
	`CREATE INDEX IF NOT EXISTS idx_catalog_analytes_scope ON catalog_analytes(tenant_id, lab_id, name);`,
	`CREATE INDEX IF NOT EXISTS idx_analysis_services_scope_order ON analysis_services(tenant_id, lab_id, department_id, sort_order, name);`,
	`CREATE INDEX IF NOT EXISTS idx_analysis_profile_services_order ON analysis_profile_services(profile_id, sort_order);`,
	`CREATE INDEX IF NOT EXISTS idx_sample_reference_scope_kind_order ON sample_reference_items(tenant_id, lab_id, kind, active, sort_order, name);`,
	`CREATE INDEX IF NOT EXISTS idx_catalog_snapshots_scope_version ON catalog_snapshots(tenant_id, lab_id, version);`,
	`CREATE INDEX IF NOT EXISTS idx_sample_intake_templates_scope_client ON sample_intake_templates(tenant_id, lab_id, client_id, name);`,
}

func OpenStore(statePath, _ string) (*Store, error) { return OpenSQLiteStore(statePath) }
func OpenSQLiteStore(dbPath string) (*Store, error) { return openSQLiteStore(dbPath, true) }
func OpenSQLiteStoreWithoutVerification(dbPath string) (*Store, error) {
	return openSQLiteStore(dbPath, false)
}

func openSQLiteStore(dbPath string, verify bool) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if verify {
		if err := store.VerifyAuditChain(); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("audit verification failed: %w", err)
		}
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying SQLite handle for local operator maintenance commands.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate(ctx context.Context) error {
	for _, stmt := range sqliteMigrations {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite migration: %w", err)
		}
	}
	if err := s.migrateV1AuditSchema(ctx); err != nil {
		return fmt.Errorf("sqlite v1 audit migration: %w", err)
	}
	for _, stmt := range sqlitePostMigrationIndexes {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite post-migration index: %w", err)
		}
	}
	return nil
}

func (s *Store) migrateV1AuditSchema(ctx context.Context) error {
	clientColumns, err := tableColumns(ctx, s.db, "clients")
	if err != nil {
		return err
	}
	if !clientColumns["tenant_id"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE clients ADD COLUMN tenant_id TEXT NOT NULL DEFAULT '`+DefaultTenantID+`'`); err != nil {
			return err
		}
	}
	if !clientColumns["lab_id"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE clients ADD COLUMN lab_id TEXT NOT NULL DEFAULT '`+DefaultLabID+`'`); err != nil {
			return err
		}
	}

	sampleColumns, err := tableColumns(ctx, s.db, "samples")
	if err != nil {
		return err
	}
	if !sampleColumns["tenant_id"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE samples ADD COLUMN tenant_id TEXT NOT NULL DEFAULT '`+DefaultTenantID+`'`); err != nil {
			return err
		}
	}
	if !sampleColumns["lab_id"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE samples ADD COLUMN lab_id TEXT NOT NULL DEFAULT '`+DefaultLabID+`'`); err != nil {
			return err
		}
	}
	for _, column := range []struct{ name, ddl string }{
		{"project_id", `project_id TEXT NOT NULL DEFAULT ''`},
		{"client_sample_id", `client_sample_id TEXT NOT NULL DEFAULT ''`},
		{"lab_sample_id", `lab_sample_id TEXT NOT NULL DEFAULT ''`},
		{"matrix_reference_id", `matrix_reference_id TEXT NOT NULL DEFAULT ''`},
		{"container_id", `container_id TEXT NOT NULL DEFAULT ''`},
		{"preservative_id", `preservative_id TEXT NOT NULL DEFAULT ''`},
		{"storage_location_id", `storage_location_id TEXT NOT NULL DEFAULT ''`},
		{"received_condition_id", `received_condition_id TEXT NOT NULL DEFAULT ''`},
		{"sampled_at", `sampled_at TEXT NOT NULL DEFAULT ''`},
		{"received_at", `received_at TEXT NOT NULL DEFAULT ''`},
		{"priority", `priority TEXT NOT NULL DEFAULT 'routine'`},
		{"comments", `comments TEXT NOT NULL DEFAULT ''`},
	} {
		if !sampleColumns[column.name] {
			if _, err := s.db.ExecContext(ctx, `ALTER TABLE samples ADD COLUMN `+column.ddl); err != nil {
				return err
			}
		}
	}

	auditColumns, err := tableColumns(ctx, s.db, "audit_events")
	if err != nil {
		return err
	}
	legacyEntityColumns := auditColumns["entity_type"] && auditColumns["entity_id"]
	addAuditColumn := func(name, ddl string) error {
		if auditColumns[name] {
			return nil
		}
		_, err := s.db.ExecContext(ctx, `ALTER TABLE audit_events ADD COLUMN `+ddl)
		return err
	}
	for _, column := range []struct{ name, ddl string }{
		{"event_id", `event_id TEXT NOT NULL DEFAULT ''`},
		{"tenant_id", `tenant_id TEXT NOT NULL DEFAULT '` + DefaultTenantID + `'`},
		{"lab_id", `lab_id TEXT NOT NULL DEFAULT '` + DefaultLabID + `'`},
		{"actor_json", `actor_json TEXT NOT NULL DEFAULT '{}'`},
		{"resource_json", `resource_json TEXT NOT NULL DEFAULT '{}'`},
		{"outcome", `outcome TEXT NOT NULL DEFAULT 'allowed'`},
		{"reason", `reason TEXT NOT NULL DEFAULT ''`},
		{"correlation_id", `correlation_id TEXT NOT NULL DEFAULT ''`},
		{"details_json", `details_json TEXT NOT NULL DEFAULT '{}'`},
	} {
		if err := addAuditColumn(column.name, column.ddl); err != nil {
			return err
		}
	}
	if legacyEntityColumns || !auditColumns["actor_json"] || !auditColumns["resource_json"] || !auditColumns["event_id"] || !auditColumns["correlation_id"] {
		if err := s.rebuildLegacyAuditRows(ctx, legacyEntityColumns); err != nil {
			return err
		}
	}
	return nil
}

func tableColumns(ctx context.Context, db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func (s *Store) rebuildLegacyAuditRows(ctx context.Context, legacyEntityColumns bool) error {
	selectSQL := `SELECT sequence, timestamp, actor, action, details_json FROM audit_events ORDER BY sequence`
	if legacyEntityColumns {
		selectSQL = `SELECT sequence, timestamp, actor, action, details_json, entity_type, entity_id FROM audit_events ORDER BY sequence`
	}
	rows, err := s.db.QueryContext(ctx, selectSQL)
	if err != nil {
		return err
	}
	defer rows.Close()
	type legacyAuditRow struct {
		sequence    int64
		timestamp   string
		actor       string
		action      string
		detailsJSON string
		entityType  string
		entityID    string
	}
	legacyRows := []legacyAuditRow{}
	for rows.Next() {
		var row legacyAuditRow
		if legacyEntityColumns {
			if err := rows.Scan(&row.sequence, &row.timestamp, &row.actor, &row.action, &row.detailsJSON, &row.entityType, &row.entityID); err != nil {
				return err
			}
		} else {
			if err := rows.Scan(&row.sequence, &row.timestamp, &row.actor, &row.action, &row.detailsJSON); err != nil {
				return err
			}
		}
		legacyRows = append(legacyRows, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	previousHash := ""
	lastHash := ""
	for _, row := range legacyRows {
		parsed, err := parseTime(row.timestamp)
		if err != nil {
			return err
		}
		resourceType := strings.TrimSpace(row.entityType)
		if resourceType == "" {
			resourceType = "legacy"
		}
		resourceID := strings.TrimSpace(row.entityID)
		if resourceID == "" {
			resourceID = fmt.Sprintf("legacy-%06d", row.sequence)
		}
		actor := legacyActorContextFromString(row.actor, defaultScope(), fmt.Sprintf("audit-%06d", row.sequence))
		event := AuditEvent{EventID: fmt.Sprintf("audit-%06d", row.sequence), TenantID: DefaultTenantID, LabID: DefaultLabID, Timestamp: parsed, Actor: actor.UserID, ActorContext: actor, Resource: AuditResource{Type: resourceType, ID: resourceID}, Action: row.action, Outcome: AuditOutcomeAllowed, CorrelationID: actor.CorrelationID, Sequence: row.sequence, Details: map[string]any{}, PreviousHash: previousHash}
		if strings.TrimSpace(row.detailsJSON) != "" {
			if err := json.Unmarshal([]byte(row.detailsJSON), &event.Details); err != nil {
				return err
			}
		}
		event.Hash = hashEvent(event)
		actorJSON, err := json.Marshal(event.ActorContext)
		if err != nil {
			return err
		}
		resourceJSON, err := json.Marshal(event.Resource)
		if err != nil {
			return err
		}
		detailsJSON, err := json.Marshal(nonNilMap(event.Details))
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE audit_events SET event_id = ?, tenant_id = ?, lab_id = ?, actor = ?, actor_json = ?, resource_json = ?, outcome = ?, reason = ?, correlation_id = ?, details_json = ?, previous_hash = ?, hash = ? WHERE sequence = ?`, event.EventID, event.TenantID, event.LabID, event.Actor, string(actorJSON), string(resourceJSON), string(event.Outcome), event.Reason, event.CorrelationID, string(detailsJSON), event.PreviousHash, event.Hash, event.Sequence); err != nil {
			return err
		}
		previousHash = event.Hash
		lastHash = event.Hash
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE store_meta SET value = ? WHERE key = 'last_hash'`, lastHash); err != nil {
		return err
	}
	if len(legacyRows) > 0 {
		_, err = s.db.ExecContext(ctx, `INSERT INTO audit_checkpoints(name, sequence, hash, created_at) VALUES ('latest', ?, ?, ?) ON CONFLICT(name) DO UPDATE SET sequence = excluded.sequence, hash = excluded.hash, created_at = excluded.created_at`, legacyRows[len(legacyRows)-1].sequence, lastHash, formatTime(time.Now().UTC()))
		return err
	}
	return nil
}

func (s *Store) CreateClient(name, email string, actor ActorContext) (Client, error) {
	return s.CreateClientForScope(defaultScope(), name, email, actor)
}

func (s *Store) CreateClientForScope(scope Scope, name, email string, actor ActorContext) (Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Client{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Client{}, errors.New("client name is required")
	}
	now := time.Now().UTC()
	var client Client
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationClientCreate, actor, AuditResource{Type: "client", ID: "new"}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		next, err := nextCounter(tx, "next_client")
		if err != nil {
			return err
		}
		client = Client{ID: fmt.Sprintf("C-%05d", next), TenantID: scope.TenantID, LabID: scope.LabID, Name: name, Email: strings.TrimSpace(email), CreatedAt: now}
		if _, err := tx.Exec(`INSERT INTO clients(id, tenant_id, lab_id, name, email, created_at) VALUES (?, ?, ?, ?, ?, ?)`, client.ID, client.TenantID, client.LabID, client.Name, client.Email, formatTime(client.CreatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "client.created", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "client", ID: client.ID}, Details: map[string]any{"name": client.Name}})
	})
	if err != nil {
		return Client{}, err
	}
	if deniedErr != nil {
		return Client{}, deniedErr
	}
	return client, nil
}

func (s *Store) CreateSample(input CreateSampleInput, actor ActorContext) (Sample, error) {
	return s.CreateSampleForScope(defaultScope(), input, actor)
}

func (s *Store) CreateSampleForScope(scope Scope, input CreateSampleInput, actor ActorContext) (Sample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Sample{}, err
	}
	input.ClientID = strings.TrimSpace(input.ClientID)
	if input.ClientID == "" {
		return Sample{}, errors.New("client id is required")
	}
	now := time.Now().UTC()
	var sample Sample
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationSampleIntake, actor, AuditResource{Type: "sample", ID: "new"}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		var clientTenant, clientLab string
		if err := tx.QueryRow(`SELECT tenant_id, lab_id FROM clients WHERE id = ?`, input.ClientID).Scan(&clientTenant, &clientLab); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown client %q", input.ClientID)
			}
			return err
		}
		if clientTenant != scope.TenantID || clientLab != scope.LabID {
			return fmt.Errorf("client %q is outside requested tenant/lab scope", input.ClientID)
		}
		if clientSampleID := strings.TrimSpace(input.ClientSampleID); clientSampleID != "" {
			var exists int
			err := tx.QueryRow(`SELECT 1 FROM samples WHERE tenant_id = ? AND lab_id = ? AND client_id = ? AND client_sample_id = ?`, scope.TenantID, scope.LabID, input.ClientID, clientSampleID).Scan(&exists)
			if err == nil {
				return fmt.Errorf("client sample id %q already exists for client %q", clientSampleID, input.ClientID)
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if labSampleID := strings.TrimSpace(input.LabSampleID); labSampleID != "" {
			var exists int
			err := tx.QueryRow(`SELECT 1 FROM samples WHERE tenant_id = ? AND lab_id = ? AND lab_sample_id = ?`, scope.TenantID, scope.LabID, labSampleID).Scan(&exists)
			if err == nil {
				return fmt.Errorf("lab sample id %q already exists", labSampleID)
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		resolved, err := resolveSampleIntakeTx(tx, scope, input)
		if err != nil {
			return err
		}
		next, err := nextCounter(tx, "next_sample")
		if err != nil {
			return err
		}
		sampleID := fmt.Sprintf("S-%06d", next)
		snapshot, hasSnapshot, err := currentCatalogSnapshotTx(tx, scope)
		if err != nil {
			return err
		}
		analyses, err := buildAnalysesTx(tx, scope, sampleID, input, resolved.Tests, snapshot, hasSnapshot)
		if err != nil {
			return err
		}
		sample = Sample{ID: sampleID, TenantID: scope.TenantID, LabID: scope.LabID, ClientID: input.ClientID, ProjectID: resolved.ProjectID, Project: resolved.Project, ClientSampleID: strings.TrimSpace(input.ClientSampleID), LabSampleID: strings.TrimSpace(input.LabSampleID), Matrix: resolved.Matrix, MatrixReferenceID: resolved.MatrixReferenceID, ContainerID: resolved.ContainerID, PreservativeID: resolved.PreservativeID, StorageLocationID: resolved.StorageLocationID, ReceivedConditionID: resolved.ReceivedConditionID, SampledAt: input.SampledAt.UTC(), ReceivedAt: input.ReceivedAt.UTC(), Priority: normalizePriority(input.Priority), Comments: strings.TrimSpace(input.Comments), Status: StatusReceived, Analyses: analyses, CreatedAt: now, UpdatedAt: now}
		encodedAnalyses, err := json.Marshal(sample.Analyses)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO samples(id, tenant_id, lab_id, client_id, project_id, project, client_sample_id, lab_sample_id, matrix, matrix_reference_id, container_id, preservative_id, storage_location_id, received_condition_id, sampled_at, received_at, priority, comments, status, analyses_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, sample.ID, sample.TenantID, sample.LabID, sample.ClientID, sample.ProjectID, sample.Project, sample.ClientSampleID, sample.LabSampleID, sample.Matrix, sample.MatrixReferenceID, sample.ContainerID, sample.PreservativeID, sample.StorageLocationID, sample.ReceivedConditionID, formatOptionalTime(sample.SampledAt), formatOptionalTime(sample.ReceivedAt), string(sample.Priority), sample.Comments, string(sample.Status), string(encodedAnalyses), formatTime(sample.CreatedAt), formatTime(sample.UpdatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.created", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"client_id": sample.ClientID, "project_id": sample.ProjectID, "client_sample_id": sample.ClientSampleID, "lab_sample_id": sample.LabSampleID, "analysis_count": len(sample.Analyses)}})
	})
	if err != nil {
		return Sample{}, err
	}
	if deniedErr != nil {
		return Sample{}, deniedErr
	}
	return sample, nil
}

func (s *Store) TransitionSample(sampleID string, next SampleStatus, actor ActorContext) error {
	return s.TransitionSampleForScope(defaultScope(), sampleID, next, actor)
}

func (s *Store) TransitionSampleForScope(scope Scope, sampleID string, next SampleStatus, actor ActorContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return err
	}
	var deniedErr error
	txErr := s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationSampleTransition, actor, AuditResource{Type: "sample", ID: sampleID}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		sample, err := sampleByIDTx(tx, sampleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown sample %q", sampleID)
			}
			return err
		}
		if sample.TenantID != scope.TenantID || sample.LabID != scope.LabID {
			deniedErr = fmt.Errorf("sample %q is outside requested tenant/lab scope", sampleID)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.transition.requested", Outcome: AuditOutcomeDenied, Reason: "scope_mismatch", Resource: AuditResource{Type: "sample", ID: sampleID}, Details: map[string]any{"requested_status": string(next)}})
		}
		if !allowedTransition(sample.Status, next) {
			deniedErr = fmt.Errorf("transition %s -> %s is not allowed", sample.Status, next)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.transition.requested", Outcome: AuditOutcomeDenied, Reason: "transition_not_allowed", Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"from": string(sample.Status), "to": string(next)}})
		}
		previous := sample.Status
		sample.Status = next
		sample.UpdatedAt = time.Now().UTC()
		if _, err := tx.Exec(`UPDATE samples SET status = ?, updated_at = ? WHERE id = ?`, string(sample.Status), formatTime(sample.UpdatedAt), sample.ID); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.transitioned", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"from": previous, "to": next}})
	})
	if txErr != nil {
		return txErr
	}
	return deniedErr
}

func (s *Store) GetSample(id string) (Sample, bool) { return s.GetSampleForScope(defaultScope(), id) }

func (s *Store) GetSampleForScope(scope Scope, id string) (Sample, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Sample{}, false
	}
	sample, err := sampleByID(s.db, id)
	if err != nil || sample.TenantID != scope.TenantID || sample.LabID != scope.LabID {
		return Sample{}, false
	}
	return sample, true
}

func (s *Store) Clients() []Client { return s.ClientsForScope(defaultScope()) }

func (s *Store) ClientsForScope(scope Scope) []Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, lab_id, name, email, created_at FROM clients WHERE tenant_id = ? AND lab_id = ? ORDER BY id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	clients := []Client{}
	for rows.Next() {
		var client Client
		var created string
		if err := rows.Scan(&client.ID, &client.TenantID, &client.LabID, &client.Name, &client.Email, &created); err != nil {
			return nil
		}
		client.CreatedAt, _ = parseTime(created)
		clients = append(clients, client)
	}
	return clients
}

func (s *Store) Samples() []Sample { return s.SamplesForScope(defaultScope()) }

func (s *Store) SamplesForScope(scope Scope) []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	rows, err := s.db.Query(sampleSelectSQL+` FROM samples WHERE tenant_id = ? AND lab_id = ? ORDER BY id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	samples := []Sample{}
	for rows.Next() {
		sample, err := scanSample(rows)
		if err != nil {
			return nil
		}
		samples = append(samples, sample)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].ID < samples[j].ID })
	return samples
}

func (s *Store) AuditEvents(limit int) ([]AuditEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return auditEventsQuery(s.db, "", limit)
}

func (s *Store) AuditEventsForScope(scope Scope, limit int) ([]AuditEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil, err
	}
	where := fmt.Sprintf("WHERE tenant_id = %q AND lab_id = %q", sqlQuote(scope.TenantID), sqlQuote(scope.LabID))
	return auditEventsQuery(s.db, where, limit)
}

func auditEventsQuery(db *sql.DB, where string, limit int) ([]AuditEvent, error) {
	base := auditSelect + " FROM audit_events " + where + " ORDER BY sequence, rowid"
	args := []any{}
	if limit > 0 {
		base = auditSelect + " FROM (SELECT * FROM audit_events " + where + " ORDER BY sequence DESC, rowid DESC LIMIT ?) ORDER BY sequence, rowid"
		args = append(args, limit)
	}
	rows, err := db.Query(base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditEvents(rows)
}

const auditSelect = `SELECT event_id, tenant_id, lab_id, timestamp, actor, actor_json, resource_json, action, outcome, reason, correlation_id, sequence, details_json, previous_hash, hash`

func (s *Store) VerifyAuditChain() error {
	rows, err := s.db.Query(auditSelect + ` FROM audit_events ORDER BY sequence, rowid`)
	if err != nil {
		return err
	}
	events, err := scanAuditEvents(rows)
	rows.Close()
	if err != nil {
		return err
	}
	if err := VerifyAuditEvents(events); err != nil {
		return err
	}
	checkpoint, ok, err := s.latestCheckpoint()
	if err != nil {
		return err
	}
	if ok {
		if len(events) == 0 && checkpoint.Sequence != 0 {
			return fmt.Errorf("checkpoint mismatch: checkpoint sequence %d but audit stream is empty", checkpoint.Sequence)
		}
		last := AuditEvent{}
		if len(events) > 0 {
			last = events[len(events)-1]
		}
		if checkpoint.Sequence != last.Sequence || checkpoint.Hash != last.Hash {
			return fmt.Errorf("checkpoint mismatch: checkpoint sequence/hash %d/%s does not match audit tail %d/%s", checkpoint.Sequence, checkpoint.Hash, last.Sequence, last.Hash)
		}
	}
	return nil
}

func VerifyAuditEvents(events []AuditEvent) error {
	var previousHash string
	seen := map[int64]bool{}
	for i, event := range events {
		if seen[event.Sequence] {
			return fmt.Errorf("duplicate sequence %d", event.Sequence)
		}
		seen[event.Sequence] = true
		expectedSequence := int64(i + 1)
		if event.Sequence != expectedSequence {
			return fmt.Errorf("sequence gap at row %d: got %d want %d", i, event.Sequence, expectedSequence)
		}
		if err := ValidateAuditEvent(event); err != nil {
			return fmt.Errorf("malformed event at sequence %d: %w", event.Sequence, err)
		}
		if event.PreviousHash != previousHash {
			return fmt.Errorf("previous hash mismatch at sequence %d", event.Sequence)
		}
		if hashEvent(event) != event.Hash {
			return fmt.Errorf("hash mismatch at sequence %d", event.Sequence)
		}
		previousHash = event.Hash
	}
	return nil
}

func ValidateAuditEvent(event AuditEvent) error {
	if strings.TrimSpace(event.EventID) == "" {
		return errors.New("event id is required")
	}
	if strings.TrimSpace(event.TenantID) == "" {
		return errors.New("tenant id is required")
	}
	if strings.TrimSpace(event.LabID) == "" {
		return errors.New("lab id is required")
	}
	if strings.TrimSpace(event.ActorContext.UserID) == "" {
		return errors.New("actor context user id is required")
	}
	if strings.TrimSpace(event.ActorContext.RequestID) == "" {
		return errors.New("actor context request id is required")
	}
	if strings.TrimSpace(event.Resource.Type) == "" || strings.TrimSpace(event.Resource.ID) == "" {
		return errors.New("resource type and id are required")
	}
	if strings.TrimSpace(event.Action) == "" {
		return errors.New("action is required")
	}
	switch event.Outcome {
	case AuditOutcomeAllowed, AuditOutcomeDenied, AuditOutcomeFailed, AuditOutcomeSystem:
	default:
		return fmt.Errorf("invalid outcome %q", event.Outcome)
	}
	if (event.Outcome == AuditOutcomeDenied || event.Outcome == AuditOutcomeFailed) && strings.TrimSpace(event.Reason) == "" {
		return errors.New("reason is required for denied/failed events")
	}
	if strings.TrimSpace(event.CorrelationID) == "" {
		return errors.New("correlation id is required")
	}
	if event.Sequence <= 0 {
		return errors.New("sequence is required")
	}
	if strings.TrimSpace(event.Hash) == "" {
		return errors.New("hash is required")
	}
	return nil
}

func (s *Store) withTx(fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func nextCounter(tx *sql.Tx, key string) (int, error) {
	var raw string
	if err := tx.QueryRow(`SELECT value FROM store_meta WHERE key = ?`, key).Scan(&raw); err != nil {
		return 0, err
	}
	next, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE store_meta SET value = ? WHERE key = ?`, strconv.Itoa(next+1), key); err != nil {
		return 0, err
	}
	return next, nil
}

type auditWrite struct {
	Scope    Scope
	Actor    ActorContext
	Action   string
	Outcome  AuditOutcome
	Reason   string
	Resource AuditResource
	Details  map[string]any
}

func appendAuditTx(tx *sql.Tx, write auditWrite) error {
	scope, err := normalizeScope(write.Scope)
	if err != nil {
		return err
	}
	nextAudit, err := nextCounter(tx, "next_audit")
	if err != nil {
		return err
	}
	var previousHash string
	if err := tx.QueryRow(`SELECT value FROM store_meta WHERE key = 'last_hash'`).Scan(&previousHash); err != nil {
		return err
	}
	actor := normalizeActorContext(write.Actor, fmt.Sprintf("audit-%06d", nextAudit))
	event := AuditEvent{EventID: fmt.Sprintf("audit-%06d", nextAudit), TenantID: scope.TenantID, LabID: scope.LabID, Timestamp: time.Now().UTC(), Actor: actor.UserID, ActorContext: actor, Resource: write.Resource, Action: strings.TrimSpace(write.Action), Outcome: write.Outcome, Reason: strings.TrimSpace(write.Reason), CorrelationID: actor.CorrelationID, Sequence: int64(nextAudit), Details: nonNilMap(write.Details), PreviousHash: previousHash}
	event.Hash = hashEvent(event)
	if err := ValidateAuditEvent(event); err != nil {
		return err
	}
	detailsJSON, err := json.Marshal(event.Details)
	if err != nil {
		return err
	}
	actorJSON, err := json.Marshal(event.ActorContext)
	if err != nil {
		return err
	}
	resourceJSON, err := json.Marshal(event.Resource)
	if err != nil {
		return err
	}
	auditColumns, err := tableColumnsTx(tx, "audit_events")
	if err != nil {
		return err
	}
	if auditColumns["entity_type"] && auditColumns["entity_id"] {
		if _, err := tx.Exec(`INSERT INTO audit_events(event_id, tenant_id, lab_id, timestamp, actor, actor_json, resource_json, action, outcome, reason, correlation_id, sequence, details_json, previous_hash, hash, entity_type, entity_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.EventID, event.TenantID, event.LabID, formatTime(event.Timestamp), event.Actor, string(actorJSON), string(resourceJSON), event.Action, string(event.Outcome), event.Reason, event.CorrelationID, event.Sequence, string(detailsJSON), event.PreviousHash, event.Hash, event.Resource.Type, event.Resource.ID); err != nil {
			return err
		}
	} else if _, err := tx.Exec(`INSERT INTO audit_events(event_id, tenant_id, lab_id, timestamp, actor, actor_json, resource_json, action, outcome, reason, correlation_id, sequence, details_json, previous_hash, hash) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.EventID, event.TenantID, event.LabID, formatTime(event.Timestamp), event.Actor, string(actorJSON), string(resourceJSON), event.Action, string(event.Outcome), event.Reason, event.CorrelationID, event.Sequence, string(detailsJSON), event.PreviousHash, event.Hash); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE store_meta SET value = ? WHERE key = 'last_hash'`, event.Hash); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO audit_checkpoints(name, sequence, hash, created_at) VALUES ('latest', ?, ?, ?) ON CONFLICT(name) DO UPDATE SET sequence = excluded.sequence, hash = excluded.hash, created_at = excluded.created_at`, event.Sequence, event.Hash, formatTime(time.Now().UTC()))
	return err
}

func tableColumnsTx(tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func normalizeActorContext(actor ActorContext, fallbackRequestID string) ActorContext {
	if actor.RequestID == "" {
		actor.RequestID = fallbackRequestID
	}
	if actor.CorrelationID == "" {
		actor.CorrelationID = actor.RequestID
	}
	if actor.DisplayNameSnapshot == "" {
		actor.DisplayNameSnapshot = actor.UserID
	}
	return actor
}

func legacyActorContextFromString(input string, scope Scope, fallbackRequestID string) ActorContext {
	userID := strings.TrimSpace(input)
	if userID == "" {
		userID = "system"
	}
	return MustActorContext(ActorContextInput{UserID: userID, DisplayName: userID, TenantMemberships: []TenantMembership{{TenantID: scope.TenantID}}, RequestID: fallbackRequestID, CorrelationID: fallbackRequestID})
}

func (s *Store) latestCheckpoint() (AuditCheckpoint, bool, error) {
	var cp AuditCheckpoint
	var created string
	err := s.db.QueryRow(`SELECT name, sequence, hash, created_at FROM audit_checkpoints WHERE name = 'latest'`).Scan(&cp.Name, &cp.Sequence, &cp.Hash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return AuditCheckpoint{}, false, nil
	}
	if err != nil {
		return AuditCheckpoint{}, false, err
	}
	cp.CreatedAt, _ = parseTime(created)
	return cp, true, nil
}

type sampleScanner interface{ Scan(dest ...any) error }

const sampleSelectSQL = `SELECT id, tenant_id, lab_id, client_id, project_id, project, client_sample_id, lab_sample_id, matrix, matrix_reference_id, container_id, preservative_id, storage_location_id, received_condition_id, sampled_at, received_at, priority, comments, status, analyses_json, created_at, updated_at`

func sampleByID(db *sql.DB, id string) (Sample, error) {
	return sampleByIDScanner(db.QueryRow(sampleSelectSQL+` FROM samples WHERE id = ?`, id))
}
func sampleByIDTx(tx *sql.Tx, id string) (Sample, error) {
	return sampleByIDScanner(tx.QueryRow(sampleSelectSQL+` FROM samples WHERE id = ?`, id))
}

func sampleByIDScanner(row sampleScanner) (Sample, error) {
	var sample Sample
	var status, analysesJSON, priority, sampledAt, receivedAt, created, updated string
	if err := row.Scan(&sample.ID, &sample.TenantID, &sample.LabID, &sample.ClientID, &sample.ProjectID, &sample.Project, &sample.ClientSampleID, &sample.LabSampleID, &sample.Matrix, &sample.MatrixReferenceID, &sample.ContainerID, &sample.PreservativeID, &sample.StorageLocationID, &sample.ReceivedConditionID, &sampledAt, &receivedAt, &priority, &sample.Comments, &status, &analysesJSON, &created, &updated); err != nil {
		return Sample{}, err
	}
	sample.Status = SampleStatus(status)
	sample.Priority = normalizePriority(SamplePriority(priority))
	sample.SampledAt = parseOptionalTime(sampledAt)
	sample.ReceivedAt = parseOptionalTime(receivedAt)
	if err := json.Unmarshal([]byte(analysesJSON), &sample.Analyses); err != nil {
		return Sample{}, err
	}
	sample.CreatedAt, _ = parseTime(created)
	sample.UpdatedAt, _ = parseTime(updated)
	return sample, nil
}
func scanSample(rows *sql.Rows) (Sample, error) { return sampleByIDScanner(rows) }

func scanAuditEvents(rows *sql.Rows) ([]AuditEvent, error) {
	events := []AuditEvent{}
	for rows.Next() {
		var event AuditEvent
		var timestamp, actorJSON, resourceJSON, outcome, detailsJSON string
		if err := rows.Scan(&event.EventID, &event.TenantID, &event.LabID, &timestamp, &event.Actor, &actorJSON, &resourceJSON, &event.Action, &outcome, &event.Reason, &event.CorrelationID, &event.Sequence, &detailsJSON, &event.PreviousHash, &event.Hash); err != nil {
			return nil, err
		}
		parsed, err := parseTime(timestamp)
		if err != nil {
			return nil, fmt.Errorf("malformed timestamp at sequence %d: %w", event.Sequence, err)
		}
		event.Timestamp = parsed
		if err := json.Unmarshal([]byte(actorJSON), &event.ActorContext); err != nil {
			return nil, fmt.Errorf("malformed actor_json at sequence %d: %w", event.Sequence, err)
		}
		if err := json.Unmarshal([]byte(resourceJSON), &event.Resource); err != nil {
			return nil, fmt.Errorf("malformed resource_json at sequence %d: %w", event.Sequence, err)
		}
		event.Outcome = AuditOutcome(outcome)
		if err := json.Unmarshal([]byte(detailsJSON), &event.Details); err != nil {
			return nil, fmt.Errorf("malformed details_json at sequence %d: %w", event.Sequence, err)
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func hashEvent(event AuditEvent) string {
	type canonical struct {
		EventID       string         `json:"event_id"`
		TenantID      string         `json:"tenant_id"`
		LabID         string         `json:"lab_id"`
		Timestamp     string         `json:"timestamp"`
		Actor         string         `json:"actor"`
		ActorContext  ActorContext   `json:"actor_context"`
		Resource      AuditResource  `json:"resource"`
		Action        string         `json:"action"`
		Outcome       AuditOutcome   `json:"outcome"`
		Reason        string         `json:"reason,omitempty"`
		CorrelationID string         `json:"correlation_id"`
		Sequence      int64          `json:"sequence"`
		Details       map[string]any `json:"details,omitempty"`
		PreviousHash  string         `json:"previous_hash"`
	}
	encoded, _ := json.Marshal(canonical{EventID: event.EventID, TenantID: event.TenantID, LabID: event.LabID, Timestamp: formatTime(event.Timestamp), Actor: event.Actor, ActorContext: event.ActorContext, Resource: event.Resource, Action: event.Action, Outcome: event.Outcome, Reason: event.Reason, CorrelationID: event.CorrelationID, Sequence: event.Sequence, Details: nonNilMap(event.Details), PreviousHash: event.PreviousHash})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func nonNilMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}
func formatTime(t time.Time) string           { return t.UTC().Format(time.RFC3339Nano) }
func parseTime(raw string) (time.Time, error) { return time.Parse(time.RFC3339Nano, raw) }
func allowedTransition(current, next SampleStatus) bool {
	order := map[SampleStatus]SampleStatus{StatusReceived: StatusInPrep, StatusInPrep: StatusInAnalysis, StatusInAnalysis: StatusInReview, StatusInReview: StatusReleased}
	return order[current] == next
}
func sqlQuote(value string) string { return strings.ReplaceAll(value, "'", "''") }
