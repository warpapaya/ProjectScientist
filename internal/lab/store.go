package lab

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
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

type State struct {
	NextClient int               `json:"next_client"`
	NextSample int               `json:"next_sample"`
	NextAudit  int64             `json:"next_audit"`
	Clients    map[string]Client `json:"clients"`
	Samples    map[string]Sample `json:"samples"`
	LastHash   string            `json:"last_hash"`
}

type Store struct {
	mu        sync.Mutex
	statePath string
	auditPath string
	state     State
}

func OpenStore(statePath, auditPath string) (*Store, error) {
	store := &Store{statePath: statePath, auditPath: auditPath}
	store.state = State{NextClient: 1, NextSample: 1, NextAudit: 1, Clients: map[string]Client{}, Samples: map[string]Sample{}}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(auditPath), 0o755); err != nil {
		return nil, err
	}
	if content, err := os.ReadFile(statePath); err == nil && len(strings.TrimSpace(string(content))) > 0 {
		if err := json.Unmarshal(content, &store.state); err != nil {
			return nil, fmt.Errorf("decode state: %w", err)
		}
		if store.state.Clients == nil {
			store.state.Clients = map[string]Client{}
		}
		if store.state.Samples == nil {
			store.state.Samples = map[string]Sample{}
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return store, nil
}

func (s *Store) CreateClient(name, email, actor string) (Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return Client{}, errors.New("client name is required")
	}
	now := time.Now().UTC()
	client := Client{ID: fmt.Sprintf("C-%05d", s.state.NextClient), Name: name, Email: strings.TrimSpace(email), CreatedAt: now}
	s.state.NextClient++
	s.state.Clients[client.ID] = client
	if err := s.audit(actor, "client.created", "client", client.ID, map[string]any{"name": client.Name}); err != nil {
		return Client{}, err
	}
	return client, s.save()
}

func (s *Store) CreateSample(input CreateSampleInput, actor string) (Sample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.Clients[input.ClientID]; !ok {
		return Sample{}, fmt.Errorf("unknown client %q", input.ClientID)
	}
	if strings.TrimSpace(input.Project) == "" {
		return Sample{}, errors.New("project is required")
	}
	if len(input.Tests) == 0 {
		return Sample{}, errors.New("at least one analysis is required")
	}
	now := time.Now().UTC()
	sampleID := fmt.Sprintf("S-%06d", s.state.NextSample)
	s.state.NextSample++
	analyses := make([]Analysis, 0, len(input.Tests))
	for i, test := range input.Tests {
		test = strings.TrimSpace(test)
		if test == "" {
			continue
		}
		analyses = append(analyses, Analysis{ID: fmt.Sprintf("%s-A%02d", sampleID, i+1), Name: test})
	}
	if len(analyses) == 0 {
		return Sample{}, errors.New("at least one non-empty analysis is required")
	}
	sample := Sample{ID: sampleID, ClientID: input.ClientID, Project: strings.TrimSpace(input.Project), Matrix: strings.TrimSpace(input.Matrix), Status: StatusReceived, Analyses: analyses, CreatedAt: now, UpdatedAt: now}
	s.state.Samples[sample.ID] = sample
	if err := s.audit(actor, "sample.created", "sample", sample.ID, map[string]any{"client_id": sample.ClientID, "analysis_count": len(sample.Analyses)}); err != nil {
		return Sample{}, err
	}
	return sample, s.save()
}

func (s *Store) TransitionSample(sampleID string, next SampleStatus, actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sample, ok := s.state.Samples[sampleID]
	if !ok {
		return fmt.Errorf("unknown sample %q", sampleID)
	}
	if !allowedTransition(sample.Status, next) {
		return fmt.Errorf("transition %s -> %s is not allowed", sample.Status, next)
	}
	previous := sample.Status
	sample.Status = next
	sample.UpdatedAt = time.Now().UTC()
	s.state.Samples[sample.ID] = sample
	if err := s.audit(actor, "sample.transitioned", "sample", sample.ID, map[string]any{"from": previous, "to": next}); err != nil {
		return err
	}
	return s.save()
}

func (s *Store) GetSample(id string) (Sample, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sample, ok := s.state.Samples[id]
	return sample, ok
}

func (s *Store) Clients() []Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	clients := make([]Client, 0, len(s.state.Clients))
	for _, client := range s.state.Clients {
		clients = append(clients, client)
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i].ID < clients[j].ID })
	return clients
}

func (s *Store) Samples() []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	samples := make([]Sample, 0, len(s.state.Samples))
	for _, sample := range s.state.Samples {
		samples = append(samples, sample)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].ID < samples[j].ID })
	return samples
}

func (s *Store) AuditEvents(limit int) ([]AuditEvent, error) {
	content, err := os.ReadFile(s.auditPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return nil, nil
	}
	start := 0
	if limit > 0 && len(lines) > limit {
		start = len(lines) - limit
	}
	events := make([]AuditEvent, 0, len(lines)-start)
	for _, line := range lines[start:] {
		var event AuditEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) audit(actor, action, entityType, entityID string, details map[string]any) error {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}
	event := AuditEvent{Sequence: s.state.NextAudit, Timestamp: time.Now().UTC(), Actor: actor, Action: action, EntityType: entityType, EntityID: entityID, Details: details, PreviousHash: s.state.LastHash}
	event.Hash = hashEvent(event)
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(s.auditPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	s.state.LastHash = event.Hash
	s.state.NextAudit++
	return nil
}

func (s *Store) save() error {
	encoded, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.statePath + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.statePath)
}

func hashEvent(event AuditEvent) string {
	copy := event
	copy.Hash = ""
	encoded, _ := json.Marshal(copy)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
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
