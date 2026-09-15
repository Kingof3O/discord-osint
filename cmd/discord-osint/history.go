package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"discord-osint/internal/store"

	"github.com/spf13/cobra"
)

var (
	historyJSON bool
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View history and progress of past OSINT search runs",
	Long:  `Displays a summary table of all previous search runs stored in the local SQLite database.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		runs, err := s.ListRuns(ctx)
		if err != nil {
			return fmt.Errorf("failed to list runs: %w", err)
		}

		if historyJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(runs)
		}

		if len(runs) == 0 {
			fmt.Println("No past search runs found in database.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "RUN ID\tTARGET\tUSER ID\tTAG\tSTATUS\tPROGRESS\tCONFIRMED\tCANDIDATES\tCREATED")
		for _, r := range runs {
			progress := fmt.Sprintf("%d/%d", r.CompletedServers, r.TotalServers)
			if r.TotalServers == 0 {
				progress = "0/0"
			}
			createdStr := r.CreatedAt.Format("2006-01-02 15:04:05")
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\n",
				r.RunID,
				r.TargetUsername,
				r.TargetUserID,
				r.Tag,
				r.Status,
				progress,
				r.ConfirmedMatches,
				r.CandidateMatches,
				createdStr,
			)
		}
		return w.Flush()
	},
}

func init() {
	historyCmd.Flags().BoolVar(&historyJSON, "json", false, "Output history in JSON format")
	rootCmd.AddCommand(historyCmd)
}
