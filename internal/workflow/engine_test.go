package workflow

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"discord-osint/internal/config"
	"discord-osint/internal/store"
	"discord-osint/internal/target"
)

type testPromptUI struct {
	confirmed bool
}

func (m *testPromptUI) DisplayCandidate(c target.Candidate, warning string) {}
func (m *testPromptUI) DisplayUnresolved(username string, warning string)   {}
func (m *testPromptUI) SelectCandidate(candidates []target.Candidate) (int, error) {
	return 0, nil
}
func (m *testPromptUI) Confirm(prompt string) (bool, error) {
	return m.confirmed, nil
}

func TestEngine_RunLifecycle_Mock(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.sqlite")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New failed: %v", err)
	}
	defer s.Close()

	cfg := config.DefaultConfig()
	cfg.JoinDelaySec = [2]int{0, 0} // zero delay for fast test

	var out bytes.Buffer
	ui := &testPromptUI{confirmed: true}

	engine := NewEngine(cfg, s, nil, nil, nil, nil, nil, ui, &out)

	params := SearchParams{
		Tag:         "gaming",
		Username:    "target_user",
		DirectCodes: []string{"invite-alpha", "invite-beta"},
		Limit:       2,
		AutoConfirm: true,
	}

	runID, err := engine.Run(context.Background(), params)
	if err != nil {
		t.Fatalf("engine.Run failed: %v", err)
	}

	if runID == "" {
		t.Fatalf("expected non-empty runID")
	}

	// Verify run was recorded in store
	run, err := s.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if run.Tag != "gaming" || run.TargetUsername != "target_user" {
		t.Errorf("mismatched run record: %+v", run)
	}

	// Verify scans were recorded
	scans, err := s.GetRunGuildScans(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRunGuildScans failed: %v", err)
	}
	if len(scans) != 2 {
		t.Errorf("expected 2 guild scans, got %d", len(scans))
	}
}
