package export

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"discord-osint/internal/store"
)

// ExportOptions controls export output directory and formats.
type ExportOptions struct {
	OutputDir string
	Formats   []string // "json", "csv", "md"
}

// ExportArtifacts generates hits.json, observations.json, messages.csv, and report.md from SQLite store.
func ExportArtifacts(ctx context.Context, s *store.Store, runID string, opts ExportOptions) error {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to load run: %w", err)
	}

	scans, err := s.GetRunGuildScans(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to load guild scans: %w", err)
	}

	obs, err := s.GetRunObservations(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to load observations: %w", err)
	}

	msgs, err := s.GetRunMessages(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to load messages: %w", err)
	}

	outDir := opts.OutputDir
	if outDir == "" {
		outDir = "."
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	// 1. Export hits.json
	hitsPath := filepath.Join(outDir, "hits.json")
	hitsData, err := json.MarshalIndent(scans, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal hits.json: %w", err)
	}
	if err := os.WriteFile(hitsPath, hitsData, 0644); err != nil {
		return fmt.Errorf("failed to write hits.json: %w", err)
	}

	// 2. Export observations.json
	obsPath := filepath.Join(outDir, "observations.json")
	obsData, err := json.MarshalIndent(obs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal observations.json: %w", err)
	}
	if err := os.WriteFile(obsPath, obsData, 0644); err != nil {
		return fmt.Errorf("failed to write observations.json: %w", err)
	}

	// 3. Export messages.csv (with RFC4180 CSV escaping)
	csvPath := filepath.Join(outDir, "messages.csv")
	csvFile, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("failed to create messages.csv: %w", err)
	}
	defer csvFile.Close()

	guildNames := make(map[string]string)
	for _, s := range scans {
		if s.GuildID != "" && s.GuildName != "" {
			guildNames[s.GuildID] = s.GuildName
		}
	}

	w := csv.NewWriter(csvFile)
	header := []string{
		"run_id", "guild_id", "guild_name", "channel_id", "channel_name", "message_id",
		"author_id", "author_username", "content",
		"timestamp", "collected_at", "collector_version",
		"acquisition_method", "coverage_status",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, m := range msgs {
		gName := m.GuildName
		if gName == "" {
			gName = guildNames[m.GuildID]
		}
		chName := m.ChannelName
		row := []string{
			m.RunID, m.GuildID, gName, m.ChannelID, chName, m.MessageID,
			m.AuthorID, m.AuthorUsername, m.Content,
			m.Timestamp.Format(time.RFC3339), m.CollectedAt.Format(time.RFC3339),
			m.CollectorVersion, m.AcquisitionMethod, m.CoverageStatus,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("csv flush error: %w", err)
	}

	// 4. Export report.md
	reportPath := filepath.Join(outDir, "report.md")
	reportMD := GenerateMarkdownReport(run, scans, obs, msgs)
	if err := os.WriteFile(reportPath, []byte(reportMD), 0644); err != nil {
		return fmt.Errorf("failed to write report.md: %w", err)
	}

	// 5. Export report.html (offline standalone HTML report)
	htmlPath := filepath.Join(outDir, "report.html")
	reportHTML := GenerateHTMLReport(run, scans, obs, msgs)
	if err := os.WriteFile(htmlPath, []byte(reportHTML), 0644); err != nil {
		return fmt.Errorf("failed to write report.html: %w", err)
	}

	return nil
}
