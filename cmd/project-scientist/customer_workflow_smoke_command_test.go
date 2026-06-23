package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestCustomerWorkflowSmokeMatrixCommandWritesArtifacts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "project-scientist.db")
	outDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := run([]string{
		"project-scientist",
		"customer-workflow", "smoke-matrix",
		"--db", dbPath,
		"--fixture", filepath.Join("..", "..", "fixtures", "golden_migration_dataset.json"),
		"--gap-report", filepath.Join("..", "..", "docs", "customer-workflow-gap-report.md"),
		"--out", outDir,
		"--command-output", "go test ./... PASS; docker/http smoke PASS",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("customer workflow smoke command failed: %v stderr=%s", err, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{"customer workflow smoke-matrix ok", "lanes=3", "green=", "yellow=", "red=", "matrix="} {
		if !strings.Contains(out, want) {
			t.Fatalf("command output missing %q: %s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "production-ready") {
		t.Fatalf("command output must not use production-ready language: %s", out)
	}

	matrixPath := strings.TrimSpace(out[strings.LastIndex(out, "matrix=")+len("matrix="):])
	var matrix lab.CustomerWorkflowSmokeMatrix
	raw, err := os.ReadFile(matrixPath)
	if err != nil {
		t.Fatalf("read matrix artifact %q: %v", matrixPath, err)
	}
	if err := json.Unmarshal(raw, &matrix); err != nil {
		t.Fatalf("decode matrix json: %v", err)
	}
	if len(matrix.Lanes) != 3 || matrix.Lanes[0].ReportPackage.Binding == "" {
		t.Fatalf("unexpected matrix artifact: %#v", matrix)
	}
}
