package main

import (
	"context"
	"fmt"
	"strings"

	"discord-osint/internal/export"
	"discord-osint/internal/store"

	"github.com/spf13/cobra"
)

var (
	exportRunID     string
	exportOutputDir string
	exportFormat    string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export investigation findings, evidence, and report from SQLite store",
	Long: `Export hits.json, observations.json, messages.csv, and human-readable report.md
for a specified run ID.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if exportRunID == "" {
			return fmt.Errorf("--run-id is required")
		}

		ctx := context.Background()

		dbPath := cfg.DatabasePath
		if dbPath == "" {
			dbPath = "run.sqlite"
		}
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer s.Close()

		if exportOutputDir == "" {
			exportOutputDir = fmt.Sprintf("exports_%s", exportRunID)
		}

		formats := strings.Split(exportFormat, ",")
		opts := export.ExportOptions{
			OutputDir: exportOutputDir,
			Formats:   formats,
		}

		if err := export.ExportArtifacts(ctx, s, exportRunID, opts); err != nil {
			return fmt.Errorf("failed to export artifacts: %w", err)
		}

		fmt.Printf("[+] Successfully exported run %s to %s/\n", exportRunID, exportOutputDir)
		fmt.Printf("    - %s/hits.json\n", exportOutputDir)
		fmt.Printf("    - %s/observations.json\n", exportOutputDir)
		fmt.Printf("    - %s/messages.csv\n", exportOutputDir)
		fmt.Printf("    - %s/report.md\n", exportOutputDir)
		return nil
	},
}

func init() {
	exportCmd.Flags().StringVar(&exportRunID, "run-id", "", "Run ID to export")
	exportCmd.Flags().StringVarP(&exportOutputDir, "output", "o", "", "Output directory for exported files")
	exportCmd.Flags().StringVar(&exportFormat, "format", "json,csv,md", "Comma-separated list of formats (json,csv,md)")
	rootCmd.AddCommand(exportCmd)
}
