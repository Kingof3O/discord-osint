package main

import (
	"context"
	"fmt"
	"os"

	"discord-osint/internal/target"

	"github.com/spf13/cobra"
)

var (
	verifyUsername string
	verifyUserID   string
	verifyYes      bool
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify target identity preflight (strictly read-only)",
	Long: `Verify target identity prior to running search or investigation.
Guaranteed strictly read-only: creates no files, writes no database rows, and makes no state changes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// If a Discord token is available, use the live resolver; otherwise use mock/fallback
		var resolver target.Resolver
		if cfg != nil && cfg.DiscordToken != "" {
			resolver = target.NewDiscordHTTPResolver(cfg.DiscordToken, nil)
		} else {
			resolver = target.NewMockResolver()
		}

		ui := target.NewTerminalUI()

		in := target.VerifyInput{
			TargetUserID:   verifyUserID,
			TargetUsername: verifyUsername,
			AutoConfirm:    verifyYes,
		}

		confirmed, err := target.VerifyTarget(ctx, in, ui, resolver)
		if err != nil {
			return err
		}

		fmt.Fprintln(os.Stdout, "Target identity confirmed successfully (read-only preflight).")
		fmt.Fprintf(os.Stdout, "Ready for search with target: %s (ID: %s)\n", confirmed.TargetUsername, confirmed.TargetUserID)
		return nil
	},
}

func init() {
	verifyCmd.Flags().StringVarP(&verifyUsername, "username", "u", "", "Discord username handle (e.g. john.doe)")
	verifyCmd.Flags().StringVarP(&verifyUserID, "user-id", "i", "", "Discord user snowflake ID")
	verifyCmd.Flags().BoolVarP(&verifyYes, "yes", "y", false, "Skip interactive confirmation prompt")
	rootCmd.AddCommand(verifyCmd)
}
