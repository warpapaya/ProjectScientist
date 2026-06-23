package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestMVPVerticalSliceCommandRunsLocalSyntheticFlow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "project-scientist.db")
	var stdout, stderr bytes.Buffer
	err := run([]string{"project-scientist", "mvp", "vertical-slice", "--db", dbPath}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("mvp vertical-slice command failed: %v stderr=%s", err, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"mvp vertical-slice ok", "sample=", "report_artifact=", "denied_controls=3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("command output missing %q: %s", want, out)
		}
	}
}
