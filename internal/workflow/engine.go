package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"time"

	"discord-osint/internal/captcha"
	"discord-osint/internal/config"
	"discord-osint/internal/disboard"
	"discord-osint/internal/discord"
	"discord-osint/internal/export"
	"discord-osint/internal/matcher"
	"discord-osint/internal/store"
	"discord-osint/internal/target"

	"go.uber.org/zap"
)

// SearchParams contains runtime inputs for a search run.
type SearchParams struct {
	Tag         string
	UserID      string
	Username    string
	Limit       int
	InvitesFile string
	DirectCodes []string
	AutoConfirm bool
	StayAfterHit bool
}

// Engine coordinates the end-to-end OSINT search lifecycle.
type Engine struct {
	cfg           *config.Config
	store         *store.Store
	discordClient *discord.Client
	gateway       *discord.GatewaySession
	captchaSolver captcha.Solver
	disboard      *disboard.Scraper
	logger        *zap.Logger
	ui            target.PromptUI
	writer        io.Writer
}

// NewEngine creates an initialized workflow engine.
func NewEngine(
	cfg *config.Config,
	s *store.Store,
	client *discord.Client,
	gw *discord.GatewaySession,
	solver captcha.Solver,
	scraper *disboard.Scraper,
	log *zap.Logger,
	ui target.PromptUI,
	w io.Writer,
) *Engine {
	if ui == nil {
		ui = target.NewTerminalUI()
	}
	if w == nil {
		w = os.Stdout
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &Engine{
		cfg:           cfg,
		store:         s,
		discordClient: client,
		gateway:       gw,
		captchaSolver: solver,
		disboard:      scraper,
		logger:        log,
		ui:            ui,
		writer:        w,
	}
}

// Run executes the complete search workflow from preflight to export.
func (e *Engine) Run(ctx context.Context, params SearchParams) (string, error) {
	// 1. Pre-flight burner health check (if client available)
	if e.discordClient != nil {
		me, err := e.discordClient.GetMe(ctx)
		if err != nil {
			return "", fmt.Errorf("pre-flight burner account check failed: %w", err)
		}
		e.logger.Info("Burner account verified", zap.String("username", me.Username), zap.String("id", me.ID))
	}

	// 2. Target Verification Preflight (read-only)
	var resolver target.Resolver
	if e.discordClient != nil {
		resolver = target.NewDiscordHTTPResolver(e.cfg.DiscordToken, nil)
	} else {
		resolver = target.NewMockResolver()
	}

	in := target.VerifyInput{
		TargetUserID:   params.UserID,
		TargetUsername: params.Username,
		AutoConfirm:    params.AutoConfirm,
	}

	confirmedTarget, err := target.VerifyTarget(ctx, in, e.ui, resolver)
	if err != nil {
		return "", fmt.Errorf("target verification aborted: %w", err)
	}

	// 3. Atomically Create Run in Store
	runID := fmt.Sprintf("run_%d", time.Now().Unix())
	tag := params.Tag
	if tag == "" {
		tag = "direct_invites"
	}
	if err := e.store.CreateRunAtomic(ctx, runID, tag, confirmedTarget); err != nil {
		return "", fmt.Errorf("failed to create run: %w", err)
	}
	fmt.Fprintf(e.writer, "[+] Run initialized with ID: %s\n", runID)

	// 4. Server Discovery
	var servers []disboard.DiscoveredServer
	if params.InvitesFile != "" {
		fmt.Fprintf(e.writer, "[*] Loading invites from file: %s\n", params.InvitesFile)
		loaded, err := disboard.LoadInvitesFromFile(params.InvitesFile, tag)
		if err != nil {
			return runID, fmt.Errorf("failed to load invites file: %w", err)
		}
		servers = append(servers, loaded...)
	}

	if len(params.DirectCodes) > 0 {
		direct := disboard.ParseDirectInvites(params.DirectCodes, tag)
		servers = append(servers, direct...)
	}

	// If no direct invites given, scrape Disboard
	if len(servers) == 0 && e.disboard != nil && params.Tag != "" {
		fmt.Fprintf(e.writer, "[*] Discovering servers from Disboard (tag: %s, limit: %d)...\n", params.Tag, params.Limit)
		discovered, err := e.disboard.Discover(ctx, params.Tag, params.Limit)
		if err != nil {
			e.logger.Warn("Disboard scraping encountered an error", zap.Error(err))
			fmt.Fprintf(e.writer, "[!] Disboard warning: %v\n", err)
		}
		servers = append(servers, discovered...)
	}

	if len(servers) == 0 {
		return runID, errors.New("no servers discovered to scan")
	}

	// Persist discovered servers to store
	var serverRecords []store.ServerRecord
	for _, s := range servers {
		serverRecords = append(serverRecords, store.ServerRecord{
			Tag:                    s.Tag,
			InviteCode:             s.InviteCode,
			GuildID:                s.GuildID,
			GuildName:              s.GuildName,
			ApproximateMemberCount: s.ApproximateMemberCount,
			DiscoveredAt:           s.DiscoveredAt,
		})
	}
	_ = e.store.SaveDiscoveredServers(ctx, serverRecords)
	fmt.Fprintf(e.writer, "[+] Discovered %d candidate servers. Beginning walk...\n\n", len(servers))

	// 5. Walk Servers
	limit := params.Limit
	if limit <= 0 {
		limit = 30
	}
	scannedCount := 0

	for _, srv := range servers {
		if scannedCount >= limit {
			break
		}

		select {
		case <-ctx.Done():
			return runID, ctx.Err()
		default:
		}

		scannedCount++
		fmt.Fprintf(e.writer, "[%d/%d] Inspecting server: %s (Invite: %s)\n",
			scannedCount, len(servers), srv.GuildName, srv.InviteCode)

		e.processServer(ctx, runID, srv, confirmedTarget, params.StayAfterHit)

		// Delay between servers (jitter)
		if scannedCount < limit && scannedCount < len(servers) {
			delayMin := e.cfg.JoinDelaySec[0]
			delayMax := e.cfg.JoinDelaySec[1]
			if delayMax < delayMin {
				delayMax = delayMin
			}
			waitSec := delayMin
			if diff := delayMax - delayMin; diff > 0 {
				waitSec += rand.Intn(diff)
			}
			if waitSec > 0 {
				fmt.Fprintf(e.writer, "[*] Waiting %ds cooldown before next join...\n\n", waitSec)
				time.Sleep(time.Duration(waitSec) * time.Second)
			}
		}
	}

	// 5.5 Mark run status complete
	if err := e.store.UpdateRunStatus(ctx, runID, "complete"); err != nil {
		e.logger.Warn("Failed to update run status to complete", zap.Error(err))
	}

	// 6. Export Artifacts
	outOpts := export.ExportOptions{
		OutputDir: fmt.Sprintf("exports_%s", runID),
		Formats:   []string{"json", "csv", "md"},
	}
	if err := export.ExportArtifacts(ctx, e.store, runID, outOpts); err != nil {
		e.logger.Error("Failed to export artifacts", zap.Error(err))
	} else {
		fmt.Fprintf(e.writer, "\n[+] Scan run %s complete. Artifacts exported to: %s/\n", runID, outOpts.OutputDir)
		fmt.Fprintf(e.writer, "    - %s/hits.json\n", outOpts.OutputDir)
		fmt.Fprintf(e.writer, "    - %s/observations.json\n", outOpts.OutputDir)
		fmt.Fprintf(e.writer, "    - %s/messages.csv\n", outOpts.OutputDir)
		fmt.Fprintf(e.writer, "    - %s/report.md\n", outOpts.OutputDir)
	}

	return runID, nil
}

func (e *Engine) processServer(
	ctx context.Context,
	runID string,
	srv disboard.DiscoveredServer,
	confirmedTarget target.ConfirmedTarget,
	stayAfterHit bool,
) {
	startTime := time.Now().UTC()
	guildID := srv.GuildID
	if guildID == "" {
		guildID = "invite_" + srv.InviteCode
	}
	scan := store.GuildScanRecord{
		RunID:           runID,
		GuildID:         guildID,
		GuildName:       srv.GuildName,
		InviteCode:      srv.InviteCode,
		ScanStatus:      "running",
		BestMatchStatus: "unknown",
		MemberCoverage:  "partial",
		MessageCoverage: "partial",
		StartedAt:       startTime,
	}

	if e.discordClient == nil {
		// Mock/test mode without live client
		scan.ScanStatus = "complete"
		scan.MemberCoverage = "complete"
		scan.BestMatchStatus = "not_found"
		scan.CompletedAt = time.Now().UTC()
		_ = e.store.RecordGuildScanTransaction(ctx, scan, nil, nil)
		return
	}

	// 1. Resolve Invite metadata (read-only)
	meta, err := e.discordClient.ResolveInvite(ctx, srv.InviteCode)
	if err != nil {
		e.logger.Warn("Failed to resolve invite", zap.String("code", srv.InviteCode), zap.Error(err))
		scan.ScanStatus = "error"
		scan.MemberStopReason = "invite_resolve_failed"
		scan.CompletedAt = time.Now().UTC()
		_ = e.store.RecordGuildScanTransaction(ctx, scan, nil, nil)
		return
	}

	if meta.Guild != nil {
		scan.GuildID = meta.Guild.ID
		scan.GuildName = meta.Guild.Name
	}

	// 2. Join Guild with CAPTCHA Handling
	var captchaToken, captchaRqToken string
	for attempt := 0; attempt < 2; attempt++ {
		joinResp, joinErr := e.discordClient.JoinGuild(ctx, srv.InviteCode, captchaToken, captchaRqToken)
		if joinErr != nil {
			var capErr *discord.CaptchaChallengeError
			if errors.As(joinErr, &capErr) {
				if e.captchaSolver == nil {
					scan.ScanStatus = "blocked"
					scan.MemberStopReason = "captcha_required_no_solver"
					scan.CompletedAt = time.Now().UTC()
					_ = e.store.RecordGuildScanTransaction(ctx, scan, nil, nil)
					return
				}

				fmt.Fprintf(e.writer, "[!] CAPTCHA required for %s. Opening interactive solver...\n", scan.GuildName)
				sol, err := e.captchaSolver.Solve(ctx, capErr.Challenge)
				if err != nil {
					e.logger.Warn("CAPTCHA solving failed or skipped", zap.Error(err))
					scan.ScanStatus = "blocked"
					scan.MemberStopReason = "captcha_unsolved"
					scan.CompletedAt = time.Now().UTC()
					_ = e.store.RecordGuildScanTransaction(ctx, scan, nil, nil)
					return
				}
				captchaToken = sol.Token
				captchaRqToken = sol.RqToken
				continue // retry join with token
			}

			// Other join error
			scan.ScanStatus = "blocked"
			scan.MemberStopReason = "join_failed"
			scan.CompletedAt = time.Now().UTC()
			_ = e.store.RecordGuildScanTransaction(ctx, scan, nil, nil)
			return
		}

		if joinResp != nil && joinResp.Guild != nil {
			scan.GuildID = joinResp.Guild.ID
			scan.GuildName = joinResp.Guild.Name
		}
		break
	}

	defer func() {
		if !stayAfterHit && scan.GuildID != "" {
			_ = e.discordClient.LeaveGuild(ctx, scan.GuildID)
		}
	}()

	// 3. Two-Tier Member Discovery
	approxMembers := 0
	if meta.Guild != nil {
		approxMembers = meta.Guild.ApproximateMemberCount
	}

	discovery, err := discord.DiscoverMembers(ctx, e.discordClient, e.gateway, scan.GuildID, confirmedTarget, approxMembers, false)
	if err != nil {
		e.logger.Warn("Member discovery failed", zap.Error(err))
	}

	scan.MemberCoverage = discovery.Coverage
	scan.MembersExamined = discovery.ExaminedCount
	scan.MemberStopReason = discovery.StopReason

	// 4. Target Matching
	var observations []matcher.MatchObservation
	for _, gm := range discovery.Members {
		if obs, matched := matcher.MatchTarget(confirmedTarget, gm, "targeted_search"); matched {
			observations = append(observations, obs)
			if obs.MatchStatus == matcher.MatchConfirmed {
				scan.BestMatchStatus = "confirmed"
				scan.BestMatchReason = string(obs.MatchReason)
				scan.BestObservedUserID = obs.ObservedUserID
				scan.BestObservedUsername = obs.ObservedUsername
			} else if obs.MatchStatus == matcher.MatchCandidate && scan.BestMatchStatus != "confirmed" {
				scan.BestMatchStatus = "candidate"
				scan.BestMatchReason = string(obs.MatchReason)
				scan.BestObservedUserID = obs.ObservedUserID
				scan.BestObservedUsername = obs.ObservedUsername
			}
		}
	}

	if len(observations) == 0 && scan.MemberCoverage == "complete" {
		scan.BestMatchStatus = "not_found"
	}

	// 5. Evidence Message Gathering (if match found)
	var messages []store.MessageRecord
	if scan.BestMatchStatus == "confirmed" || scan.BestMatchStatus == "candidate" {
		fmt.Fprintf(e.writer, "[+] MATCH FOUND in %s: %s (%s)\n", scan.GuildName, scan.BestMatchStatus, scan.BestMatchReason)

		targetQuery := confirmedTarget.TargetUsername
		targetID := confirmedTarget.TargetUserID
		if scan.BestObservedUserID != "" {
			targetID = scan.BestObservedUserID
		}

		collectedMsgs, err := e.discordClient.SearchGuildMessages(ctx, scan.GuildID, targetID, targetQuery)
		if err == nil {
			for _, m := range collectedMsgs {
				messages = append(messages, store.MessageRecord{
					RunID:             runID,
					GuildID:           scan.GuildID,
					ChannelID:         m.ChannelID,
					MessageID:         m.ID,
					AuthorID:          m.Author.ID,
					AuthorUsername:    m.Author.Username,
					Content:           m.Content,
					Timestamp:         m.Timestamp,
					CollectedAt:       time.Now().UTC(),
					CollectorVersion:  "v3.3",
					AcquisitionMethod: "guild_search_api",
					CoverageStatus:    "bounded",
				})
			}
			scan.MessageCoverage = "bounded"
			scan.MessagesExamined = len(messages)
		}
	}

	scan.ScanStatus = "complete"
	scan.CompletedAt = time.Now().UTC()

	// 6. Checkpoint atomically
	_ = e.store.RecordGuildScanTransaction(ctx, scan, observations, messages)
}
