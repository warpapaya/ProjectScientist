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

type Client struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type Analysis struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Method string `json:"method,omitempty"`
	Result string `json:"result,omitempty"`
	Units  string `json:"units,omitempty"`
}

type Sample struct {
	ID        string       `json:"id"`
	ClientID  string       `json:"client_id"`
	Project   string       `json:"project"`
	Matrix    string       `json:"matrix"`
	Status    SampleStatus `json:"status"`
	Analyses  []Analysis   `json:"analyses"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type CreateSampleInput struct {
	ClientID string   `json:"client_id"`
	Project  string   `json:"project"`
	Matrix   string   `json:"matrix"`
	Tests    []string `json:"tests"`
}

type AuditEvent struct {
	Sequence     int64          `json:"sequence"`
	Timestamp    time.Time      `json:"timestamp"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	EntityType   string         `json:"entity_type"`
	EntityID     string         `json:"entity_id"`
	Details      map[string]any `json:"details,omitempty"`
	PreviousHash string         `json:"previous_hash"`
	Hash         string         `json:"hash"`
}

type Store struct {
	mu sync.Mutex
	db *sql.DB
}

type StoreRepository interface {
	CreateClient(name, email, actor string) (Client, error)
	CreateSample(input CreateSampleInput, actor string) (Sample, error)
	TransitionSample(sampleID string, next SampleStatus, actor string) error
	GetSample(id string) (Sample, bool)
	Clients() []Client
	Samples() []Sample
	AuditEvents(limit int) ([]AuditEvent, error)
	Close() error
}

const sqliteSchemaVersion = 1

var sqliteMigrations = []string{
	`PRAGMA journal_mode = WAL;`,
	`PRAGMA foreign_keys = ON;`,
	`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS store_meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS clients (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL CHECK (length(trim(name)) > 0),
		email TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS samples (
		id TEXT PRIMARY KEY,
		client_id TEXT NOT NULL REFERENCES clients(id),
		project TEXT NOT NULL CHECK (length(trim(project)) > 0),
		matrix TEXT NOT NULL,
		status TEXT NOT NULL,
		analyses_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS audit_events (
		sequence INTEGER PRIMARY KEY,
		timestamp TEXT NOT NULL,
		actor TEXT NOT NULL,
		action TEXT NOT NULL CHECK (length(trim(action)) > 0),
		entity_type TEXT NOT NULL CHECK (length(trim(entity_type)) > 0),
		entity_id TEXT NOT NULL CHECK (length(trim(entity_id)) > 0),
		details_json TEXT NOT NULL,
		previous_hash TEXT NOT NULL,
		hash TEXT NOT NULL UNIQUE
	);`,
	`CREATE INDEX IF NOT EXISTS idx_samples_client_id ON samples(client_id);`,
	`INSERT OR IGNORE INTO store_meta(key, value) VALUES
		('next_client', '1'),
		('next_sample', '1'),
		('next_audit', '1'),
		('last_hash', '');`,
	`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));`,
}

func OpenStore(statePath, _ string) (*Store, error) {
	return OpenSQLiteStore(statePath)
}

func OpenSQLiteStore(dbPath string) (*Store, error) {
	return openSQLiteStore(dbPath, true)
}

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

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	for _, stmt := range sqliteMigrations {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite migration: %w", err)
		}
	}
	return nil
}

func (s *Store) CreateClient(name, email, actor string) (Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return Client{}, errors.New("client name is required")
	}
	now := time.Now().UTC()
	var client Client
	err := s.withTx(func(tx *sql.Tx) error {
		next, err := nextCounter(tx, "next_client")
		if err != nil {
			return err
		}
		client = Client{ID: fmt.Sprintf("C-%05d", next), Name: name, Email: strings.TrimSpace(email), CreatedAt: now}
		if _, err := tx.Exec(`INSERT INTO clients(id, name, email, created_at) VALUES (?, ?, ?, ?)`, client.ID, client.Name, client.Email, formatTime(client.CreatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, actor, "client.created", "client", client.ID, map[string]any{"name": client.Name})
	})
	if err != nil {
		return Client{}, err
	}
	return client, nil
}

func (s *Store) CreateSample(input CreateSampleInput, actor string) (Sample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(input.Project) == "" {
		return Sample{}, errors.New("project is required")
	}
	if len(input.Tests) == 0 {
		return Sample{}, errors.New("at least one analysis is required")
	}
	now := time.Now().UTC()
	var sample Sample
	err := s.withTx(func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM clients WHERE id = ?`, input.ClientID).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown client %q", input.ClientID)
			}
			return err
		}
		next, err := nextCounter(tx, "next_sample")
		if err != nil {
			return err
		}
		sampleID := fmt.Sprintf("S-%06d", next)
		analyses := make([]Analysis, 0, len(input.Tests))
		for i, test := range input.Tests {
			test = strings.TrimSpace(test)
			if test == "" {
				continue
			}
			analyses = append(analyses, Analysis{ID: fmt.Sprintf("%s-A%02d", sampleID, i+1), Name: test})
		}
		if len(analyses) == 0 {
			return errors.New("at least one non-empty analysis is required")
		}
		sample = Sample{ID: sampleID, ClientID: input.ClientID, Project: strings.TrimSpace(input.Project), Matrix: strings.TrimSpace(input.Matrix), Status: StatusReceived, Analyses: analyses, CreatedAt: now, UpdatedAt: now}
		encodedAnalyses, err := json.Marshal(sample.Analyses)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO samples(id, client_id, project, matrix, status, analyses_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, sample.ID, sample.ClientID, sample.Project, sample.Matrix, string(sample.Status), string(encodedAnalyses), formatTime(sample.CreatedAt), formatTime(sample.UpdatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, actor, "sample.created", "sample", sample.ID, map[string]any{"client_id": sample.ClientID, "analysis_count": len(sample.Analyses)})
	})
	if err != nil {
		return Sample{}, err
	}
	return sample, nil
}

func (s *Store) TransitionSample(sampleID string, next SampleStatus, actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withTx(func(tx *sql.Tx) error {
		sample, err := sampleByIDTx(tx, sampleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown sample %q", sampleID)
			}
			return err
		}
		if !allowedTransition(sample.Status, next) {
			return fmt.Errorf("transition %s -> %s is not allowed", sample.Status, next)
		}
		previous := sample.Status
		sample.Status = next
		sample.UpdatedAt = time.Now().UTC()
		if _, err := tx.Exec(`UPDATE samples SET status = ?, updated_at = ? WHERE id = ?`, string(sample.Status), formatTime(sample.UpdatedAt), sample.ID); err != nil {
			return err
		}
		return appendAuditTx(tx, actor, "sample.transitioned", "sample", sample.ID, map[string]any{"from": previous, "to": next})
	})
}

func (s *Store) GetSample(id string) (Sample, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sample, err := sampleByID(s.db, id)
	return sample, err == nil
}

func (s *Store) Clients() []Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, name, email, created_at FROM clients ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	clients := []Client{}
	for rows.Next() {
		var client Client
		var created string
		if err := rows.Scan(&client.ID, &client.Name, &client.Email, &created); err != nil {
			return nil
		}
		client.CreatedAt, _ = parseTime(created)
		clients = append(clients, client)
	}
	return clients
}

func (s *Store) Samples() []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, client_id, project, matrix, status, analyses_json, created_at, updated_at FROM samples ORDER BY id`)
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
	query := `SELECT sequence, timestamp, actor, action, entity_type, entity_id, details_json, previous_hash, hash FROM audit_events ORDER BY sequence`
	args := []any{}
	if limit > 0 {
		query = `SELECT sequence, timestamp, actor, action, entity_type, entity_id, details_json, previous_hash, hash FROM (SELECT * FROM audit_events ORDER BY sequence DESC LIMIT ?) ORDER BY sequence`
		args = append(args, limit)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditEvents(rows)
}

func (s *Store) VerifyAuditChain() error {
	rows, err := s.db.Query(`SELECT sequence, timestamp, actor, action, entity_type, entity_id, details_json, previous_hash, hash FROM audit_events ORDER BY sequence`)
	if err != nil {
		return err
	}
	defer rows.Close()
	events, err := scanAuditEvents(rows)
	if err != nil {
		return err
	}
	var previousHash string
	for i, event := range events {
		expectedSequence := int64(i + 1)
		if event.Sequence != expectedSequence {
			return fmt.Errorf("sequence gap at row %d: got %d want %d", i, event.Sequence, expectedSequence)
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

func appendAuditTx(tx *sql.Tx, actor, action, entityType, entityID string, details map[string]any) error {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}
	nextAudit, err := nextCounter(tx, "next_audit")
	if err != nil {
		return err
	}
	var previousHash string
	if err := tx.QueryRow(`SELECT value FROM store_meta WHERE key = 'last_hash'`).Scan(&previousHash); err != nil {
		return err
	}
	event := AuditEvent{Sequence: int64(nextAudit), Timestamp: time.Now().UTC(), Actor: actor, Action: action, EntityType: entityType, EntityID: entityID, Details: details, PreviousHash: previousHash}
	event.Hash = hashEvent(event)
	detailsJSON, err := json.Marshal(event.Details)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO audit_events(sequence, timestamp, actor, action, entity_type, entity_id, details_json, previous_hash, hash) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.Sequence, formatTime(event.Timestamp), event.Actor, event.Action, event.EntityType, event.EntityID, string(detailsJSON), event.PreviousHash, event.Hash); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE store_meta SET value = ? WHERE key = 'last_hash'`, event.Hash)
	return err
}

type sampleScanner interface {
	Scan(dest ...any) error
}

func sampleByID(db *sql.DB, id string) (Sample, error) {
	return sampleByIDScanner(db.QueryRow(`SELECT id, client_id, project, matrix, status, analyses_json, created_at, updated_at FROM samples WHERE id = ?`, id))
}

func sampleByIDTx(tx *sql.Tx, id string) (Sample, error) {
	return sampleByIDScanner(tx.QueryRow(`SELECT id, client_id, project, matrix, status, analyses_json, created_at, updated_at FROM samples WHERE id = ?`, id))
}

func sampleByIDScanner(row sampleScanner) (Sample, error) {
	var sample Sample
	var status, analysesJSON, created, updated string
	if err := row.Scan(&sample.ID, &sample.ClientID, &sample.Project, &sample.Matrix, &status, &analysesJSON, &created, &updated); err != nil {
		return Sample{}, err
	}
	sample.Status = SampleStatus(status)
	if err := json.Unmarshal([]byte(analysesJSON), &sample.Analyses); err != nil {
		return Sample{}, err
	}
	sample.CreatedAt, _ = parseTime(created)
	sample.UpdatedAt, _ = parseTime(updated)
	return sample, nil
}

func scanSample(rows *sql.Rows) (Sample, error) {
	return sampleByIDScanner(rows)
}

func scanAuditEvents(rows *sql.Rows) ([]AuditEvent, error) {
	events := []AuditEvent{}
	for rows.Next() {
		var event AuditEvent
		var timestamp, detailsJSON string
		if err := rows.Scan(&event.Sequence, &timestamp, &event.Actor, &event.Action, &event.EntityType, &event.EntityID, &detailsJSON, &event.PreviousHash, &event.Hash); err != nil {
			return nil, err
		}
		parsed, err := parseTime(timestamp)
		if err != nil {
			return nil, err
		}
		event.Timestamp = parsed
		if err := json.Unmarshal([]byte(detailsJSON), &event.Details); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func hashEvent(event AuditEvent) string {
	copy := event
	copy.Hash = ""
	encoded, _ := json.Marshal(copy)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, raw)
}

func allowedTransition(current, next SampleStatus) bool {
	order := map[SampleStatus]SampleStatus{
		StatusReceived:   StatusInPrep,
		StatusInPrep:     StatusInAnalysis,
		StatusInAnalysis: StatusInReview,
		StatusInReview:   StatusReleased,
	}
	return order[current] == next
}
