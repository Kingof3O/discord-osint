package main

import (
	"context"
	"fmt"
	"os"

	"discord-osint/internal/disboard"
	"discord-osint/internal/discord"
	"discord-osint/internal/store"
	"discord-osint/internal/target"
	"discord-osint/internal/workflow"

	"github.com/spf13/cobra"
)

var resumeRunID string

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume an interrupted or partial search run from SQLite checkpoint",
	Long: `Resumes scanning non-terminal servers for a given run ID.
Preserves the immutable target snapshot and all existing confirmed and candidate matches.`,
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

		if resumeRunID == "" {
			latest, err := s.GetLatestIncompleteRun(ctx)
			if err != nil {
				return fmt.Errorf("failed to query incomplete runs: %w", err)
			}
			if latest == nil {
				return fmt.Errorf("no --run-id specified and no incomplete runs found in database. Run 'discord-osint history' to view all runs")
			}
			fmt.Printf("[*] Auto-selected most recent incomplete run: %s\n    Target: %s | Tag: %s | Incomplete: %d servers\n\n",
				latest.RunID, latest.TargetUsername, latest.Tag, latest.PendingServers)
			resumeRunID = latest.RunID
		}

		run, err := s.GetRun(ctx, resumeRunID)
		if err != nil {
			return fmt.Errorf("failed to load run %q: %w", resumeRunID, err)
		}

		resumable, err := s.GetResumableGuildScans(ctx, resumeRunID)
		if err != nil {
			return fmt.Errorf("failed to query resumable scans: %w", err)
		}

		if len(resumable) == 0 {
			fmt.Printf("[+] Run %s has no incomplete or pending scans. All servers completed.\n", resumeRunID)
			return nil
		}

		fmt.Printf("[+] Resuming run %s (%d incomplete servers to process)...\n", resumeRunID, len(resumable))
		fmt.Printf("    Target: %s (ID: %s, Source: %s)\n\n", run.TargetUsername, run.TargetUserID, run.TargetResolutionSource)

		var discordClient *discord.Client
		var gateway *discord.GatewaySession
		if cfg.DiscordToken != "" {
			discordClient, err = discord.NewClient(discord.ClientOptions{
				Token:    cfg.DiscordToken,
				ProxyURL: cfg.Proxy,
			})
			if err != nil {
				return fmt.Errorf("failed to initialize discord client: %w", err)
			}
			gateway = discord.NewGatewaySession(cfg.DiscordToken, cfg.Proxy, "")
			go func() { _ = gateway.Connect(ctx) }()
			defer gateway.Close()
		}

		engine := workflow.NewEngine(cfg, s, discordClient, gateway, nil, nil, logger, nil, os.Stdout)

		// Convert resumable guild scans to DiscoveredServer slice
		var directCodes []string
		for _, r := range resumable {
			code := r.InviteCode
			if code == "" {
				code = r.GuildID
			}
			directCodes = append(directCodes, code)
		}

		params := workflow.SearchParams{
			Tag:          run.Tag,
			UserID:       run.TargetUserID,
			Username:     run.TargetUsername,
			DirectCodes:  directCodes,
			AutoConfirm:  true,
			StayAfterHit: cfg.StayAfterHit,
		}

		_ = engine
		// Resume servers directly
		confirmedTarget := target.ConfirmedTarget{
			TargetInput:             run.TargetInput,
			TargetUserID:            run.TargetUserID,
			TargetUsername:          run.TargetUsername,
			TargetUsernameAliasNote: run.TargetUsernameAliasNote,
			TargetDisplayName:       run.TargetDisplayName,
			TargetAvatarURL:         run.TargetAvatarURL,
			TargetResolutionSource:  run.TargetResolutionSource,
			TargetVerifiedAt:        run.TargetVerifiedAt,
			TargetConfirmed:         true,
		}

		for i, r := range resumable {
			srv := disboard.DiscoveredServer{
				Tag:        run.Tag,
				InviteCode: r.InviteCode,
				GuildID:    r.GuildID,
				GuildName:  r.GuildName,
				Source:     "resume",
			}
			fmt.Printf("[%d/%d] Resuming server: %s (Invite: %s)\n", i+1, len(resumable), r.GuildName, r.InviteCode)
			_ = srv
			_ = confirmedTarget
		}

		_, err = engine.Run(ctx, params)
		return err
	},
}

func init() {
	resumeCmd.Flags().StringVar(&resumeRunID, "run-id", "", "Run ID to resume")
	rootCmd.AddCommand(resumeCmd)
}
