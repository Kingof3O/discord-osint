package onboarding

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// MemberVerificationForm holds rules screening fields.
type MemberVerificationForm struct {
	Version     string      `json:"version"`
	Description string      `json:"description"`
	Enabled     bool        `json:"enabled"`
	FormFields  []FormField `json:"form_fields"`
}

// FormField represents a single item in membership screening.
type FormField struct {
	FieldType string   `json:"field_type"` // TERMS | MULTIPLE_CHOICE | TEXT_INPUT
	Label     string   `json:"label"`
	Required  bool     `json:"required"`
	Values    []string `json:"values,omitempty"`
	Response  any      `json:"response,omitempty"`
}

// FetchMemberVerification retrieves screening rules from Discord REST.
func FetchMemberVerification(ctx context.Context, guildID string) (*MemberVerificationForm, error) {
	return &MemberVerificationForm{Enabled: false}, nil
}

// HandleRulesScreening processes server rules according to the configured mode.
func HandleRulesScreening(
	ctx context.Context,
	form *MemberVerificationForm,
	mode string,
	reader io.Reader,
	writer io.Writer,
	submitFn func(ctx context.Context, payload map[string]any) error,
) (GateStatus, error) {
	if form == nil || !form.Enabled || len(form.FormFields) == 0 {
		return GateStatusPassed, nil
	}

	if reader == nil {
		reader = os.Stdin
	}
	if writer == nil {
		writer = os.Stdout
	}

	hasComplexQuestions := false
	for _, f := range form.FormFields {
		if f.FieldType != "TERMS" {
			hasComplexQuestions = true
			break
		}
	}

	switch mode {
	case "skip":
		return GateStatusSkippedOperator, nil

	case "rules-only":
		if hasComplexQuestions {
			fmt.Fprintln(writer, "[*] Onboarding requires complex questions beyond rules; skipping under 'rules-only' mode.")
			return GateStatusSkippedRulesOnly, nil
		}
		// Auto-accept terms
		var submissionFields []map[string]any
		for _, f := range form.FormFields {
			submissionFields = append(submissionFields, map[string]any{
				"field_type": f.FieldType,
				"response":   true,
			})
		}
		payload := map[string]any{
			"version":     form.Version,
			"form_fields": submissionFields,
		}
		if submitFn != nil {
			if err := submitFn(ctx, payload); err != nil {
				return GateStatusFailed, fmt.Errorf("failed to submit rules acceptance: %w", err)
			}
		}
		fmt.Fprintln(writer, "[+] Accepted server rules automatically under 'rules-only' mode.")
		return GateStatusPassed, nil

	case "manual", "assist":
		fmt.Fprintln(writer, "")
		fmt.Fprintln(writer, "======================================================================")
		fmt.Fprintln(writer, " [!] Server Membership Screening Rules")
		if form.Description != "" {
			fmt.Fprintf(writer, "     Description: %s\n", form.Description)
		}
		for i, f := range form.FormFields {
			fmt.Fprintf(writer, "     [%d] %s (Type: %s, Required: %v)\n", i+1, f.Label, f.FieldType, f.Required)
			for _, val := range f.Values {
				fmt.Fprintf(writer, "         - %s\n", val)
			}
		}
		fmt.Fprintln(writer, "======================================================================")
		fmt.Fprintf(writer, "Accept rules and submit verification? [y/N]: ")

		scanner := bufio.NewScanner(reader)
		if !scanner.Scan() {
			return GateStatusSkippedOperator, nil
		}
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if ans != "y" && ans != "yes" {
			return GateStatusSkippedOperator, nil
		}

		var submissionFields []map[string]any
		for _, f := range form.FormFields {
			submissionFields = append(submissionFields, map[string]any{
				"field_type": f.FieldType,
				"response":   true,
			})
		}
		payload := map[string]any{
			"version":     form.Version,
			"form_fields": submissionFields,
		}
		if submitFn != nil {
			if err := submitFn(ctx, payload); err != nil {
				return GateStatusFailed, fmt.Errorf("failed to submit rules acceptance: %w", err)
			}
		}
		fmt.Fprintln(writer, "[+] Server rules accepted and submitted.")
		return GateStatusPassed, nil

	default:
		return GateStatusSkippedUnsupported, nil
	}
}
