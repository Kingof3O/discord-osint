package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"discord-osint/internal/captcha"
	"discord-osint/internal/disboard"
	"discord-osint/internal/discord"
	"discord-osint/internal/store"
	"discord-osint/internal/workflow"

	"github.com/spf13/cobra"
)

var (
	searchTag         string
	searchUserID      string
	searchUsername    string
	searchLimit       int
	searchInvitesFile string
	searchDirectCodes []string
	searchAutoConfirm bool
	searchStay        bool
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Execute an end-to-end OSINT search for a target user across Discord servers",
	Long: `Search Discord servers for target user presence with strict identity separation,
interactive manual CAPTCHA solving, anti-bot TLS hardening, two-tier member discovery,
and comprehensive audit provenance.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Setup signal handling for graceful Ctrl+C shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			fmt.Fprintln(os.Stderr, "\n[!] Interrupt received. Gracefully saving checkpoint and shutting down...")
			cancel()
		}()

		// 1. Initialize SQLite store
		dbPath := cfg.DatabasePath
		if dbPath == "" {
			dbPath = "run.sqlite"
		}
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database at %s: %w", dbPath, err)
		}
		defer s.Close()

		// 2. Initialize Discord REST & Gateway clients (if token provided)
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
			go func() {
				_ = gateway.Connect(ctx)
			}()
			defer gateway.Close()
		}

		// 3. Initialize CAPTCHA solver bridge
		var solver captcha.Solver
		if cfg.Captcha == "manual" {
			bridgeCfg := captcha.DefaultConfig()
			bridgeCfg.Port = cfg.CaptchaBridgePort
			if cfg.CaptchaTimeoutSec > 0 {
				bridgeCfg.Timeout = time.Duration(cfg.CaptchaTimeoutSec) * time.Second
			}
			solver = captcha.NewInteractiveBridge(bridgeCfg)
		}

		// 4. Initialize Disboard scraper
		scraper := disboard.NewScraper(nil, cfg.DisboardCookies)

		// 5. Initialize Workflow Engine
		engine := workflow.NewEngine(cfg, s, discordClient, gateway, solver, scraper, logger, nil, os.Stdout)

		params := workflow.SearchParams{
			Tag:          searchTag,
			UserID:       searchUserID,
			Username:     searchUsername,
			Limit:        searchLimit,
			InvitesFile:  searchInvitesFile,
			DirectCodes:  searchDirectCodes,
			AutoConfirm:  searchAutoConfirm,
			StayAfterHit: searchStay,
		}

		if params.Limit <= 0 {
			params.Limit = cfg.Limit
		}
		if params.Tag == "" {
			params.Tag = cfg.Tag
		}

		_, err = engine.Run(ctx, params)
		return err
	},
}

func init() {
	searchCmd.Flags().StringVarP(&searchTag, "tag", "t", "", "Disboard category tag (e.g. gaming, crypto)")
	searchCmd.Flags().StringVarP(&searchUsername, "username", "u", "", "Target Discord username handle")
	searchCmd.Flags().StringVarP(&searchUserID, "user-id", "i", "", "Target Discord user snowflake ID")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "l", 30, "Maximum number of servers to walk")
	searchCmd.Flags().StringVar(&searchInvitesFile, "invites-file", "", "Path to text file containing direct invite links/codes")
	searchCmd.Flags().StringSliceVar(&searchDirectCodes, "invite", nil, "Direct invite code(s) or URL(s)")
	searchCmd.Flags().BoolVarP(&searchAutoConfirm, "yes", "y", false, "Skip interactive preflight confirmation")
	searchCmd.Flags().BoolVar(&searchStay, "stay", false, "Stay in guild after hit instead of leaving")

	rootCmd.AddCommand(searchCmd)
}
