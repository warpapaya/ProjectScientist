package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

type performanceSmokeSummary struct {
	DBPath           string                   `json:"db_path"`
	Concurrency      int                      `json:"concurrency"`
	Limits           performanceSmokeLimits   `json:"limits"`
	SamplesCreated   int                      `json:"samples_created"`
	ResultsEntered   int                      `json:"results_entered"`
	ResultsAccepted  int                      `json:"results_accepted"`
	ReportsGenerated int                      `json:"reports_generated"`
	AuditEvents      int                      `json:"audit_events"`
	DurationMillis   int64                    `json:"duration_ms"`
	OpsPerSecond     float64                  `json:"ops_per_second"`
	Observations     []string                 `json:"observations"`
	Remediations     []performanceRemediation `json:"remediations,omitempty"`
}

type performanceSmokeLimits struct {
	Samples int `json:"samples"`
	Reports int `json:"reports"`
}

type performanceRemediation struct {
	Severity string `json:"severity"`
	Task     string `json:"task"`
	Reason   string `json:"reason"`
}

type performanceSmokeSample struct {
	Sample lab.Sample
	Result lab.Result
}

func smokePerformance(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("smoke performance", flag.ContinueOnError)
	fs.SetOutput(stderr)
	defaultDB := filepath.Join(os.TempDir(), fmt.Sprintf("project-scientist-performance-smoke-%d.db", time.Now().UnixNano()))
	dbPath := fs.String("db", defaultDB, "SQLite database path for the local smoke run")
	samples := fs.Int("samples", 12, "number of synthetic sample/result mutation flows")
	concurrency := fs.Int("concurrency", 4, "maximum concurrent mutation workers")
	reports := fs.Int("reports", 3, "number of released COA report artifacts to generate")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON summary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *samples < 1 {
		return errors.New("samples must be greater than zero")
	}
	if *concurrency < 1 {
		return errors.New("concurrency must be greater than zero")
	}
	if *reports < 0 {
		return errors.New("reports cannot be negative")
	}
	if *reports > *samples {
		return fmt.Errorf("reports (%d) cannot exceed samples (%d)", *reports, *samples)
	}
	summary, err := runPerformanceSmoke(*dbPath, *samples, *concurrency, *reports)
	if err != nil {
		return err
	}
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}
	fmt.Fprintf(stdout, "performance smoke ok db=%s samples=%d results=%d accepted=%d reports=%d audit_events=%d duration_ms=%d ops_per_second=%.2f\n", summary.DBPath, summary.SamplesCreated, summary.ResultsEntered, summary.ResultsAccepted, summary.ReportsGenerated, summary.AuditEvents, summary.DurationMillis, summary.OpsPerSecond)
	for _, observation := range summary.Observations {
		fmt.Fprintf(stdout, "observation: %s\n", observation)
	}
	for _, remediation := range summary.Remediations {
		fmt.Fprintf(stdout, "remediation[%s]: %s — %s\n", remediation.Severity, remediation.Task, remediation.Reason)
	}
	return nil
}

func runPerformanceSmoke(dbPath string, sampleLimit, concurrency, reportLimit int) (performanceSmokeSummary, error) {
	started := time.Now()
	store, err := lab.OpenSQLiteStore(dbPath)
	if err != nil {
		return performanceSmokeSummary{}, fmt.Errorf("open smoke store: %w", err)
	}
	defer store.Close()

	manager := performanceSmokeActor("smoke-manager", lab.RoleAdmin, lab.RoleLabManager)
	client, err := store.CreateClient("PSC Performance Smoke Synthetic Lab", "smoke@example.test", manager)
	if err != nil {
		return performanceSmokeSummary{}, fmt.Errorf("seed smoke client: %w", err)
	}

	workers := concurrency
	if workers > sampleLimit {
		workers = sampleLimit
	}
	jobs := make(chan int)
	created := make([]performanceSmokeSample, 0, sampleLimit)
	var createdMu sync.Mutex
	var firstErr error
	var errMu sync.Mutex
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				item, err := exercisePerformanceSmokeSample(store, client.ID, i)
				if err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					continue
				}
				createdMu.Lock()
				created = append(created, item)
				createdMu.Unlock()
			}
		}()
	}
	for i := 0; i < sampleLimit; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return performanceSmokeSummary{}, firstErr
	}
	sort.Slice(created, func(i, j int) bool { return created[i].Sample.ID < created[j].Sample.ID })

	releaser := performanceSmokeActor("smoke-releaser", lab.RoleReportReleaser)
	for i := 0; i < reportLimit; i++ {
		if _, err := store.GenerateCOAReportArtifact(lab.COAGenerationInput{
			SampleID: created[i].Sample.ID,
			Template: lab.COATemplate{
				ID:         "psc-smoke-coa",
				Version:    "2026.06-smoke",
				Style:      lab.COAStyleTindall,
				LabName:    "PSC Smoke Lab",
				ClientName: client.Name,
			},
			Locale:    "en-US",
			Narrative: "local performance/concurrency smoke; synthetic lab-test data only",
		}, releaser); err != nil {
			return performanceSmokeSummary{}, fmt.Errorf("generate report for %s: %w", created[i].Sample.ID, err)
		}
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		return performanceSmokeSummary{}, fmt.Errorf("read audit events: %w", err)
	}
	if err := store.VerifyAuditChain(); err != nil {
		return performanceSmokeSummary{}, fmt.Errorf("verify audit chain: %w", err)
	}

	duration := time.Since(started)
	opCount := 1 + sampleLimit*7 + reportLimit
	summary := performanceSmokeSummary{
		DBPath:           dbPath,
		Concurrency:      concurrency,
		Limits:           performanceSmokeLimits{Samples: sampleLimit, Reports: reportLimit},
		SamplesCreated:   len(created),
		ResultsEntered:   len(created),
		ResultsAccepted:  len(created),
		ReportsGenerated: reportLimit,
		AuditEvents:      len(events),
		DurationMillis:   duration.Milliseconds(),
		OpsPerSecond:     float64(opCount) / duration.Seconds(),
		Observations: []string{
			"synthetic lab-test lane only; no customer/prod mutation or customer-facing claims",
			"concurrent goroutines exercise sample mutations, result entry/review, transition audit writes, and COA report artifact generation against one SQLite store",
			"current Store serializes write paths with an in-process mutex; this smoke validates correctness and catches obvious contention/errors, not production-grade throughput",
		},
	}
	if summary.OpsPerSecond < 20 {
		summary.Remediations = append(summary.Remediations, performanceRemediation{Severity: "medium", Task: "profile serialized SQLite write path under realistic lab batch sizes", Reason: fmt.Sprintf("observed %.2f ops/sec below local smoke floor of 20 ops/sec", summary.OpsPerSecond)})
	}
	return summary, nil
}

func exercisePerformanceSmokeSample(store *lab.Store, clientID string, index int) (performanceSmokeSample, error) {
	analyst := performanceSmokeActor(fmt.Sprintf("smoke-analyst-%02d", index%4), lab.RoleAnalyst)
	reviewer := performanceSmokeActor(fmt.Sprintf("smoke-reviewer-%02d", index%4), lab.RoleReviewer)
	manager := performanceSmokeActor("smoke-manager", lab.RoleLabManager)
	sample, err := store.CreateSample(lab.CreateSampleInput{
		ClientID:       clientID,
		Project:        "PSC-RM-082 synthetic concurrency batch",
		ClientSampleID: fmt.Sprintf("PSC-RM-082-C%03d", index+1),
		LabSampleID:    fmt.Sprintf("PSC-RM-082-L%03d", index+1),
		Matrix:         "Water",
		Priority:       lab.PriorityRoutine,
		Tests:          []string{"Lead"},
	}, manager)
	if err != nil {
		return performanceSmokeSample{}, fmt.Errorf("create sample %d: %w", index, err)
	}
	lines := store.AnalysisRequestLinesForSample(sample.ID)
	if len(lines) != 1 {
		return performanceSmokeSample{}, fmt.Errorf("sample %s expected one analysis request line, got %d", sample.ID, len(lines))
	}
	result, err := store.CreateResult(lab.ResultInput{AnalysisRequestLineID: lines[0].ID, Value: float64(index+1) / 1000, RawValue: fmt.Sprintf("%.4f mg/L", float64(index+1)/1000), Unit: "mg/L", MDL: 0.001, RL: 0.005, Dilution: 1, AnalystID: analyst.UserID, InstrumentID: "PSC-SMOKE-ICP"}, analyst)
	if err != nil {
		return performanceSmokeSample{}, fmt.Errorf("create result for %s: %w", sample.ID, err)
	}
	accepted, err := store.ReviewResult(result.ID, lab.ResultReviewInput{Decision: lab.ResultDecisionAccept, Comments: "accepted by performance smoke", EnforceReviewerSeparation: true}, reviewer)
	if err != nil {
		return performanceSmokeSample{}, fmt.Errorf("accept result %s: %w", result.ID, err)
	}
	for _, status := range []lab.SampleStatus{lab.StatusInPrep, lab.StatusInAnalysis, lab.StatusInReview, lab.StatusReleased} {
		if err := store.TransitionSample(sample.ID, status, manager); err != nil {
			return performanceSmokeSample{}, fmt.Errorf("transition sample %s to %s: %w", sample.ID, status, err)
		}
	}
	refetched, ok := store.GetSample(sample.ID)
	if !ok {
		return performanceSmokeSample{}, fmt.Errorf("refetch sample %s", sample.ID)
	}
	return performanceSmokeSample{Sample: refetched, Result: accepted}, nil
}

func performanceSmokeActor(userID string, roles ...lab.Role) lab.ActorContext {
	roleStrings := make([]string, 0, len(roles))
	for _, role := range roles {
		roleStrings = append(roleStrings, string(role))
	}
	correlation := "psc-rm-082-performance-smoke"
	return lab.MustActorContext(lab.ActorContextInput{
		UserID:            strings.TrimSpace(userID),
		DisplayName:       strings.TrimSpace(userID),
		AuthProvider:      "performance-smoke",
		RequestID:         "req-" + strings.TrimSpace(userID),
		CorrelationID:     correlation,
		TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: roleStrings}},
		Roles:             roleStrings,
	})
}
