package target

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// MockUI records interactions and provides pre-programmed answers for testing.
type MockUI struct {
	DisplayedCandidate   Candidate
	DisplayedWarning     string
	DisplayedUnresolved  string
	SelectedCandidateIdx int
	ConfirmAnswer        bool
	ConfirmPrompt        string
	ConfirmCalledCount   int
}

func (m *MockUI) DisplayCandidate(c Candidate, warning string) {
	m.DisplayedCandidate = c
	m.DisplayedWarning = warning
}

func (m *MockUI) DisplayUnresolved(username string, warning string) {
	m.DisplayedUnresolved = username
	m.DisplayedWarning = warning
}

func (m *MockUI) SelectCandidate(candidates []Candidate) (int, error) {
	if m.SelectedCandidateIdx < 0 || m.SelectedCandidateIdx >= len(candidates) {
		return -1, ErrSelectionAborted
	}
	return m.SelectedCandidateIdx, nil
}

func (m *MockUI) Confirm(prompt string) (bool, error) {
	m.ConfirmPrompt = prompt
	m.ConfirmCalledCount++
	return m.ConfirmAnswer, nil
}

func TestVerifyTarget_ResolvedSingle(t *testing.T) {
	resolver := NewMockResolver()
	resolver.UsernameResolutions["john.doe"] = Resolution{
		Input:  "john.doe",
		Status: Resolved,
		Candidates: []Candidate{
			{
				UserID:      "123456789012345678",
				Username:    "john.doe",
				DisplayName: "John Doe",
				AvatarURL:   "https://cdn.discordapp.com/avatars/123/abc.png",
				Source:      "mutual-guild-cache",
				CreatedAt:   time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	ui := &MockUI{ConfirmAnswer: true}
	res, err := VerifyTarget(context.Background(), VerifyInput{TargetUsername: "john.doe"}, ui, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TargetUserID != "123456789012345678" {
		t.Errorf("expected user ID 123456789012345678, got %q", res.TargetUserID)
	}
	if res.TargetUsername != "john.doe" {
		t.Errorf("expected username john.doe, got %q", res.TargetUsername)
	}
	if !res.TargetConfirmed {
		t.Errorf("expected target to be confirmed")
	}
	if ui.ConfirmCalledCount != 1 {
		t.Errorf("expected confirm to be called once, called %d times", ui.ConfirmCalledCount)
	}
}

func TestVerifyTarget_MultipleCandidates(t *testing.T) {
	resolver := NewMockResolver()
	resolver.UsernameResolutions["john.doe"] = Resolution{
		Input:  "john.doe",
		Status: MultipleCandidates,
		Candidates: []Candidate{
			{
				UserID:      "111111111111111111",
				Username:    "john.doe",
				DisplayName: "John One",
				Source:      "guild-a",
			},
			{
				UserID:      "222222222222222222",
				Username:    "john.doe",
				DisplayName: "John Two",
				Source:      "guild-b",
			},
		},
	}

	ui := &MockUI{
		SelectedCandidateIdx: 1, // select John Two
		ConfirmAnswer:        true,
	}

	res, err := VerifyTarget(context.Background(), VerifyInput{TargetUsername: "john.doe"}, ui, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TargetUserID != "222222222222222222" {
		t.Errorf("expected user ID 222222222222222222, got %q", res.TargetUserID)
	}
	if res.TargetDisplayName != "John Two" {
		t.Errorf("expected display name John Two, got %q", res.TargetDisplayName)
	}
}

func TestVerifyTarget_Unresolved(t *testing.T) {
	resolver := NewMockResolver() // empty, returns Unresolved

	ui := &MockUI{ConfirmAnswer: true}
	res, err := VerifyTarget(context.Background(), VerifyInput{TargetUsername: "ghost_user"}, ui, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TargetUserID != "" {
		t.Errorf("expected empty user ID for unresolved, got %q", res.TargetUserID)
	}
	if res.TargetUsername != "ghost_user" {
		t.Errorf("expected normalized username 'ghost_user', got %q", res.TargetUsername)
	}
	if !strings.Contains(ui.DisplayedWarning, "subsequent matches can only ever be 'candidate', never 'confirmed'") {
		t.Errorf("expected warning about candidate-only matches, got %q", ui.DisplayedWarning)
	}
}

func TestVerifyTarget_MismatchIDAndUsername(t *testing.T) {
	resolver := NewMockResolver()
	resolver.IDResolutions["123456789012345678"] = Candidate{
		UserID:      "123456789012345678",
		Username:    "canonical.name",
		DisplayName: "Canonical",
		Source:      "users-api",
	}

	ui := &MockUI{ConfirmAnswer: true}
	input := VerifyInput{
		TargetUserID:   "123456789012345678",
		TargetUsername: "alias.name",
	}

	res, err := VerifyTarget(context.Background(), input, ui, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TargetUserID != "123456789012345678" {
		t.Errorf("expected ID 123456789012345678, got %q", res.TargetUserID)
	}
	if res.TargetUsername != "canonical.name" {
		t.Errorf("expected resolved canonical username, got %q", res.TargetUsername)
	}
	if !strings.Contains(res.TargetUsernameAliasNote, "alias.name") {
		t.Errorf("expected alias note to mention alias.name, got %q", res.TargetUsernameAliasNote)
	}
	if !strings.Contains(ui.DisplayedWarning, "differs from resolved username") {
		t.Errorf("expected mismatch warning, got %q", ui.DisplayedWarning)
	}
}

func TestVerifyTarget_Rejected(t *testing.T) {
	resolver := NewMockResolver()
	resolver.UsernameResolutions["test.user"] = Resolution{
		Input:  "test.user",
		Status: Resolved,
		Candidates: []Candidate{
			{UserID: "123456789012345678", Username: "test.user"},
		},
	}

	ui := &MockUI{ConfirmAnswer: false} // operator says No
	_, err := VerifyTarget(context.Background(), VerifyInput{TargetUsername: "test.user"}, ui, resolver)
	if !errors.Is(err, ErrTargetRejected) {
		t.Fatalf("expected ErrTargetRejected, got %v", err)
	}
}

func TestVerifyTarget_ReadOnlyGuarantee(t *testing.T) {
	// Verify that executing VerifyTarget leaves absolutely no files in the current working directory
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp: %v", err)
	}
	defer os.Chdir(origWd)

	resolver := NewMockResolver()
	resolver.UsernameResolutions["test.user"] = Resolution{
		Input:  "test.user",
		Status: Resolved,
		Candidates: []Candidate{
			{UserID: "123456789012345678", Username: "test.user"},
		},
	}

	ui := &MockUI{ConfirmAnswer: true}
	_, err := VerifyTarget(context.Background(), VerifyInput{TargetUsername: "test.user"}, ui, resolver)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("READ-ONLY GUARANTEE VIOLATION: files created in working directory: %v", names)
	}
}

func TestTerminalUI_SelectCandidate(t *testing.T) {
	candidates := []Candidate{
		{UserID: "111", Username: "alpha", Source: "src1"},
		{UserID: "222", Username: "beta", Source: "src2"},
	}

	// Test selecting 2
	input := bytes.NewBufferString("2\n")
	var output bytes.Buffer
	ui := &TerminalUI{Reader: input, Writer: &output}

	idx, err := ui.SelectCandidate(candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 1 {
		t.Errorf("expected index 1, got %d", idx)
	}

	// Test quitting with q
	inputQuit := bytes.NewBufferString("q\n")
	uiQuit := &TerminalUI{Reader: inputQuit, Writer: &output}
	_, err = uiQuit.SelectCandidate(candidates)
	if !errors.Is(err, ErrSelectionAborted) {
		t.Errorf("expected ErrSelectionAborted, got %v", err)
	}
}

func TestSnowflakeParsing(t *testing.T) {
	// Discord epoch test: ID 175928847299117063
	// ms = (175928847299117063 >> 22) + 1420070400000 = 41943040000 + 1420070400000 = 1462013440000 (2016-04-30 14:50:40 UTC)
	id := "175928847299117063"
	if !IsValidSnowflake(id) {
		t.Errorf("expected valid snowflake")
	}

	ts, err := SnowflakeToTime(id)
	if err != nil {
		t.Fatalf("failed to parse snowflake: %v", err)
	}

	expectedYear := 2016
	if ts.Year() != expectedYear {
		t.Errorf("expected year %d, got %d (ts: %s)", expectedYear, ts.Year(), ts)
	}

	if IsValidSnowflake("not-a-snowflake") {
		t.Errorf("expected invalid snowflake")
	}
}
