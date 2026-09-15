package target

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	ErrTargetRejected   = errors.New("target verification was rejected by operator")
	ErrNoTargetProvided = errors.New("neither --username nor --user-id was provided")
	ErrSelectionAborted = errors.New("candidate selection was aborted by operator")
)

// VerifyTarget executes the target verification preflight workflow according to spec §4.
// It is strictly read-only: it generates no DB records and touches no output files.
func VerifyTarget(ctx context.Context, in VerifyInput, ui PromptUI, resolver Resolver) (ConfirmedTarget, error) {
	if in.TargetUserID == "" && in.TargetUsername == "" {
		return ConfirmedTarget{}, ErrNoTargetProvided
	}

	now := time.Now().UTC()

	// Case 1: Both UserID and Username are supplied
	if in.TargetUserID != "" && in.TargetUsername != "" {
		resolved, err := resolver.ResolveUserID(ctx, in.TargetUserID)
		if err != nil {
			return ConfirmedTarget{}, fmt.Errorf("failed to resolve user ID %s: %w", in.TargetUserID, err)
		}

		normResolved := NormalizeUsername(resolved.Username)
		normSupplied := NormalizeUsername(in.TargetUsername)

		var warning string
		var aliasNote string
		if normResolved != normSupplied {
			warning = fmt.Sprintf("WARNING: supplied username %q differs from resolved username for ID %s (%q, source: %s).\nThe run will key on user ID %s (confirmed path).\nSupplied username will be kept as an alias note only.",
				in.TargetUsername, resolved.UserID, resolved.Username, resolved.Source, resolved.UserID)
			aliasNote = fmt.Sprintf("supplied alias: %s", in.TargetUsername)
		}

		ui.DisplayCandidate(resolved, warning)

		prompt := "Use this target? [y/N]"
		if warning != "" {
			prompt = "Continue with ID-keyed target? [y/N]"
		}

		if !in.AutoConfirm {
			ok, err := ui.Confirm(prompt)
			if err != nil {
				return ConfirmedTarget{}, err
			}
			if !ok {
				return ConfirmedTarget{}, ErrTargetRejected
			}
		}

		return ConfirmedTarget{
			TargetInput:             fmt.Sprintf("id:%s username:%s", in.TargetUserID, in.TargetUsername),
			TargetUserID:            resolved.UserID,
			TargetUsername:          resolved.Username,
			TargetUsernameAliasNote: aliasNote,
			TargetDisplayName:       resolved.DisplayName,
			TargetAvatarURL:         resolved.AvatarURL,
			TargetResolutionSource:  resolved.Source,
			TargetVerifiedAt:        now,
			TargetConfirmed:         true,
		}, nil
	}

	// Case 2: Only UserID is supplied
	if in.TargetUserID != "" {
		resolved, err := resolver.ResolveUserID(ctx, in.TargetUserID)
		if err != nil {
			return ConfirmedTarget{}, fmt.Errorf("failed to resolve user ID %s: %w", in.TargetUserID, err)
		}

		ui.DisplayCandidate(resolved, "")

		if !in.AutoConfirm {
			ok, err := ui.Confirm("Use this target? [y/N]")
			if err != nil {
				return ConfirmedTarget{}, err
			}
			if !ok {
				return ConfirmedTarget{}, ErrTargetRejected
			}
		}

		return ConfirmedTarget{
			TargetInput:            in.TargetUserID,
			TargetUserID:           resolved.UserID,
			TargetUsername:         resolved.Username,
			TargetDisplayName:      resolved.DisplayName,
			TargetAvatarURL:        resolved.AvatarURL,
			TargetResolutionSource: resolved.Source,
			TargetVerifiedAt:       now,
			TargetConfirmed:        true,
		}, nil
	}

	// Case 3: Only Username is supplied
	res, err := resolver.ResolveUsername(ctx, in.TargetUsername)
	if err != nil {
		return ConfirmedTarget{}, fmt.Errorf("failed to resolve username %q: %w", in.TargetUsername, err)
	}

	switch res.Status {
	case Resolved:
		if len(res.Candidates) == 0 {
			return ConfirmedTarget{}, fmt.Errorf("resolution status is resolved but returned 0 candidates")
		}
		c := res.Candidates[0]
		ui.DisplayCandidate(c, "")

		if !in.AutoConfirm {
			ok, err := ui.Confirm("Use this target? [y/N]")
			if err != nil {
				return ConfirmedTarget{}, err
			}
			if !ok {
				return ConfirmedTarget{}, ErrTargetRejected
			}
		}

		return ConfirmedTarget{
			TargetInput:            in.TargetUsername,
			TargetUserID:           c.UserID,
			TargetUsername:         c.Username,
			TargetDisplayName:      c.DisplayName,
			TargetAvatarURL:        c.AvatarURL,
			TargetResolutionSource: c.Source,
			TargetVerifiedAt:       now,
			TargetConfirmed:        true,
		}, nil

	case MultipleCandidates:
		idx, err := ui.SelectCandidate(res.Candidates)
		if err != nil {
			return ConfirmedTarget{}, err
		}
		if idx < 0 || idx >= len(res.Candidates) {
			return ConfirmedTarget{}, ErrSelectionAborted
		}
		selected := res.Candidates[idx]
		ui.DisplayCandidate(selected, "")

		if !in.AutoConfirm {
			ok, err := ui.Confirm(fmt.Sprintf("Selected [%d] %s (%s). Use this target? [y/N]", idx+1, selected.Username, selected.UserID))
			if err != nil {
				return ConfirmedTarget{}, err
			}
			if !ok {
				return ConfirmedTarget{}, ErrTargetRejected
			}
		}

		return ConfirmedTarget{
			TargetInput:            in.TargetUsername,
			TargetUserID:           selected.UserID,
			TargetUsername:         selected.Username,
			TargetDisplayName:      selected.DisplayName,
			TargetAvatarURL:        selected.AvatarURL,
			TargetResolutionSource: selected.Source,
			TargetVerifiedAt:       now,
			TargetConfirmed:        true,
		}, nil

	case Unresolved:
		warning := "WARNING: username could not be resolved to a user ID.\nIf you continue, subsequent matches can only ever be 'candidate', never 'confirmed'."
		ui.DisplayUnresolved(in.TargetUsername, warning)

		if !in.AutoConfirm {
			ok, err := ui.Confirm("Use this username-only target? [y/N]")
			if err != nil {
				return ConfirmedTarget{}, err
			}
			if !ok {
				return ConfirmedTarget{}, ErrTargetRejected
			}
		}

		return ConfirmedTarget{
			TargetInput:            in.TargetUsername,
			TargetUserID:           "", // username-only
			TargetUsername:         NormalizeUsername(in.TargetUsername),
			TargetDisplayName:      in.TargetUsername,
			TargetResolutionSource: "unresolved",
			TargetVerifiedAt:       now,
			TargetConfirmed:        true,
		}, nil

	default:
		return ConfirmedTarget{}, fmt.Errorf("unexpected resolution status: %s", res.Status)
	}
}

// TerminalUI provides interactive CLI prompts to the operator via stdout and stdin.
type TerminalUI struct {
	Reader io.Reader
	Writer io.Writer
}

// NewTerminalUI creates a new terminal UI bound to standard in/out.
func NewTerminalUI() *TerminalUI {
	return &TerminalUI{
		Reader: os.Stdin,
		Writer: os.Stdout,
	}
}

func (t *TerminalUI) DisplayCandidate(c Candidate, warning string) {
	fmt.Fprintln(t.Writer, "")
	fmt.Fprintln(t.Writer, "Target verification")
	fmt.Fprintln(t.Writer, "===================")
	fmt.Fprintf(t.Writer, "Username:      %s\n", c.Username)
	fmt.Fprintf(t.Writer, "Display name:  %s\n", c.DisplayName)
	fmt.Fprintf(t.Writer, "User ID:       %s\n", c.UserID)
	if !c.CreatedAt.IsZero() {
		fmt.Fprintf(t.Writer, "Created At:    %s\n", c.CreatedAt.Format(time.RFC3339))
	}
	avatar := c.AvatarURL
	if avatar == "" {
		avatar = "<unavailable>"
	}
	fmt.Fprintf(t.Writer, "Avatar:        %s\n", avatar)
	fmt.Fprintf(t.Writer, "Resolver:      %s\n", c.Source)
	fmt.Fprintln(t.Writer, "Resolution:    resolved")

	if warning != "" {
		fmt.Fprintln(t.Writer, "")
		fmt.Fprintln(t.Writer, warning)
	}
	fmt.Fprintln(t.Writer, "")
}

func (t *TerminalUI) DisplayUnresolved(username string, warning string) {
	fmt.Fprintln(t.Writer, "")
	fmt.Fprintln(t.Writer, "Target verification")
	fmt.Fprintln(t.Writer, "===================")
	fmt.Fprintf(t.Writer, "Username:      %s\n", username)
	fmt.Fprintln(t.Writer, "Resolution:    unresolved")

	if warning != "" {
		fmt.Fprintln(t.Writer, "")
		fmt.Fprintln(t.Writer, warning)
	}
	fmt.Fprintln(t.Writer, "")
}

func (t *TerminalUI) SelectCandidate(candidates []Candidate) (int, error) {
	fmt.Fprintln(t.Writer, "")
	fmt.Fprintf(t.Writer, "Multiple candidates found (%d candidates):\n\n", len(candidates))
	for i, c := range candidates {
		disp := c.DisplayName
		if disp == "" {
			disp = c.Username
		}
		fmt.Fprintf(t.Writer, "  [%d] %s / %s / %s (source: %s)\n", i+1, c.Username, disp, c.UserID, c.Source)
	}
	fmt.Fprintln(t.Writer, "")

	scanner := bufio.NewScanner(t.Reader)
	attempts := 0
	for attempts < 2 {
		fmt.Fprintf(t.Writer, "Select target [1-%d] or 'q' to quit: ", len(candidates))
		if !scanner.Scan() {
			return -1, ErrSelectionAborted
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "q" || input == "Q" {
			return -1, ErrSelectionAborted
		}

		num, err := strconv.Atoi(input)
		if err == nil && num >= 1 && num <= len(candidates) {
			return num - 1, nil
		}
		attempts++
		if attempts < 2 {
			fmt.Fprintln(t.Writer, "Invalid selection. Please try again.")
		}
	}

	return -1, ErrSelectionAborted
}

func (t *TerminalUI) Confirm(prompt string) (bool, error) {
	fmt.Fprintf(t.Writer, "%s ", prompt)
	scanner := bufio.NewScanner(t.Reader)
	if !scanner.Scan() {
		return false, nil
	}
	input := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return input == "y" || input == "yes", nil
}
