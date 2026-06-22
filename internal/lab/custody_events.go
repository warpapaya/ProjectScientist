package lab

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type CustodyEventType string

const (
	CustodyReceived    CustodyEventType = "received"
	CustodyTransferred CustodyEventType = "transferred"
	CustodySplit       CustodyEventType = "split"
	CustodyStored      CustodyEventType = "stored"
	CustodyDisposed    CustodyEventType = "disposed"
	CustodyReturned    CustodyEventType = "returned"
)

var ErrCustodyHistoryImmutable = errors.New("custody history is immutable")

type CustodyEvent struct {
	ID         string           `json:"id"`
	TenantID   string           `json:"tenant_id"`
	LabID      string           `json:"lab_id"`
	SampleID   string           `json:"sample_id"`
	Type       CustodyEventType `json:"type"`
	Actor      ActorContext     `json:"actor"`
	OccurredAt time.Time        `json:"occurred_at"`
	Location   string           `json:"location"`
	Reason     string           `json:"reason"`
	Sequence   int64            `json:"sequence"`
	CreatedAt  time.Time        `json:"created_at"`
}

type CustodyEventInput struct {
	SampleID   string           `json:"sample_id"`
	Type       CustodyEventType `json:"type"`
	OccurredAt time.Time        `json:"occurred_at"`
	Location   string           `json:"location"`
	Reason     string           `json:"reason"`
}

func (s *Store) RecordCustodyEvent(input CustodyEventInput, actor ActorContext) (CustodyEvent, error) {
	return s.RecordCustodyEventForScope(defaultScope(), input, actor)
}

func (s *Store) RecordCustodyEventForScope(scope Scope, input CustodyEventInput, actor ActorContext) (CustodyEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return CustodyEvent{}, err
	}
	input.SampleID = strings.TrimSpace(input.SampleID)
	input.Location = strings.TrimSpace(input.Location)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.SampleID == "" {
		return CustodyEvent{}, errors.New("sample id is required")
	}
	if err := validateCustodyEventInput(input); err != nil {
		return CustodyEvent{}, err
	}
	var event CustodyEvent
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationSampleCustody, actor, AuditResource{Type: "sample", ID: input.SampleID}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		sample, err := sampleByIDForScopeTx(tx, scope, input.SampleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				deniedErr = fmt.Errorf("unknown sample %q", input.SampleID)
				return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.custody.record.requested", Outcome: AuditOutcomeDenied, Reason: "sample_not_found", Resource: AuditResource{Type: "sample", ID: input.SampleID}, Details: map[string]any{"custody_type": string(input.Type), "location": input.Location}})
			}
			return err
		}
		nextID, err := nextCounter(tx, "next_custody_event")
		if err != nil {
			return err
		}
		sequence, err := nextCustodySequenceTx(tx, sample.ID)
		if err != nil {
			return err
		}
		occurredAt := input.OccurredAt.UTC()
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}
		createdAt := time.Now().UTC()
		custodyActor := normalizeActorContext(actor, fmt.Sprintf("custody-%06d", nextID))
		event = CustodyEvent{ID: fmt.Sprintf("CE-%06d", nextID), TenantID: scope.TenantID, LabID: scope.LabID, SampleID: sample.ID, Type: input.Type, Actor: custodyActor, OccurredAt: occurredAt, Location: input.Location, Reason: input.Reason, Sequence: sequence, CreatedAt: createdAt}
		actorJSON, err := json.Marshal(event.Actor)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO custody_events(id, tenant_id, lab_id, sample_id, type, actor_json, occurred_at, location, reason, sequence, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.ID, event.TenantID, event.LabID, event.SampleID, string(event.Type), actorJSON, formatTime(event.OccurredAt), event.Location, event.Reason, event.Sequence, formatTime(event.CreatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.custody.recorded", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"custody_event_id": event.ID, "custody_type": string(event.Type), "location": event.Location, "reason": event.Reason, "custody_sequence": event.Sequence}})
	})
	if err != nil {
		return CustodyEvent{}, err
	}
	if deniedErr != nil {
		return CustodyEvent{}, deniedErr
	}
	return event, nil
}

func (s *Store) UpdateCustodyEvent(sampleID, eventID string, input CustodyEventInput, actor ActorContext) error {
	return ErrCustodyHistoryImmutable
}

func validateCustodyEventInput(input CustodyEventInput) error {
	switch input.Type {
	case CustodyReceived, CustodyTransferred, CustodySplit, CustodyStored, CustodyDisposed, CustodyReturned:
	default:
		return fmt.Errorf("unsupported custody event type %q", input.Type)
	}
	if strings.TrimSpace(input.Location) == "" {
		return errors.New("location is required")
	}
	if strings.TrimSpace(input.Reason) == "" {
		return errors.New("reason is required")
	}
	return nil
}

func nextCustodySequenceTx(tx *sql.Tx, sampleID string) (int64, error) {
	var current sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(sequence) FROM custody_events WHERE sample_id = ?`, sampleID).Scan(&current); err != nil {
		return 0, err
	}
	if !current.Valid {
		return 1, nil
	}
	return current.Int64 + 1, nil
}

func custodyEventsForSampleDB(db *sql.DB, scope Scope, sampleID string) ([]CustodyEvent, error) {
	rows, err := db.Query(`SELECT id, tenant_id, lab_id, sample_id, type, actor_json, occurred_at, location, reason, sequence, created_at FROM custody_events WHERE tenant_id = ? AND lab_id = ? AND sample_id = ? ORDER BY sequence`, scope.TenantID, scope.LabID, sampleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCustodyEvents(rows)
}

func scanCustodyEvents(rows *sql.Rows) ([]CustodyEvent, error) {
	events := []CustodyEvent{}
	for rows.Next() {
		var event CustodyEvent
		var eventType, actorJSON, occurredAt, createdAt string
		if err := rows.Scan(&event.ID, &event.TenantID, &event.LabID, &event.SampleID, &eventType, &actorJSON, &occurredAt, &event.Location, &event.Reason, &event.Sequence, &createdAt); err != nil {
			return nil, err
		}
		event.Type = CustodyEventType(eventType)
		if err := json.Unmarshal([]byte(actorJSON), &event.Actor); err != nil {
			return nil, fmt.Errorf("malformed custody actor_json for %s: %w", event.ID, err)
		}
		parsedOccurredAt, err := parseTime(occurredAt)
		if err != nil {
			return nil, fmt.Errorf("malformed custody occurred_at for %s: %w", event.ID, err)
		}
		event.OccurredAt = parsedOccurredAt
		event.CreatedAt, _ = parseTime(createdAt)
		events = append(events, event)
	}
	return events, rows.Err()
}
