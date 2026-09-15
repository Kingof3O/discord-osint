package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"discord-osint/internal/ai"
	"discord-osint/internal/store"

	"github.com/spf13/cobra"
)

var (
	triageRunID   string
	triageModel   string
	triageBaseURL string
	triageAPIKey  string
)

var triageCmd = &cobra.Command{
	Use:   "triage",
	Short: "Generate an AI threat intelligence and behavioral triage summary for a run",
	Long: `Synthesizes collected message evidence and community presence for a given run ID
into an executive triage summary (triage.md) using OpenCode Zen, OpenAI, or any OpenAI-compatible API.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if triageRunID == "" {
			return fmt.Errorf("--run-id is required")
		}

		apiKey := triageAPIKey
		if apiKey == "" {
			apiKey = cfg.OpenCodeAPIKey
		}
		if apiKey == "" && cfg.OpenAIAPIKey != "" {
			apiKey = cfg.OpenAIAPIKey
		}
		if apiKey == "" {
			return fmt.Errorf("AI API key not configured (set via --api-key flag, OPENCODE_API_KEY env, or in config)")
		}

		baseURL := triageBaseURL
		if baseURL == "" {
			baseURL = cfg.OpenCodeBaseURL
		}
		if baseURL == "" {
			baseURL = "https://opencode.ai/zen/v1"
		}

		model := triageModel
		if model == "" {
			model = cfg.AIModel
		}
		if model == "" {
			return fmt.Errorf("AI model not configured (set via --model flag, AI_MODEL/OPENCODE_MODEL env, or ai_model in config. E.g. --model musespark-1.3)")
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

		run, err := s.GetRun(ctx, triageRunID)
		if err != nil {
			return fmt.Errorf("failed to load run %q: %w", triageRunID, err)
		}

		msgs, err := s.GetRunMessages(ctx, triageRunID)
		if err != nil {
			return fmt.Errorf("failed to load messages: %w", err)
		}

		fmt.Printf("[*] Generating AI triage analysis for target: %s (Run: %s, Model: %s, Messages: %d)...\n",
			run.TargetUsername, triageRunID, model, len(msgs))

		aiClient := ai.NewClient(ai.ClientOptions{
			APIKey:  apiKey,
			BaseURL: baseURL,
			Model:   model,
		})

		summary, err := aiClient.TriageEvidence(ctx, run.TargetUsername, msgs)
		if err != nil {
			return fmt.Errorf("AI triage analysis failed: %w", err)
		}

		outDir := fmt.Sprintf("exports_%s", triageRunID)
		_ = os.MkdirAll(outDir, 0755)
		triagePath := filepath.Join(outDir, "triage.md")
		content := fmt.Sprintf("# AI Intelligence Triage Report\n\n**Run ID:** `%s`\n**Target:** `%s`\n**Model:** `%s`\n\n%s\n",
			triageRunID, run.TargetUsername, model, summary)
		_ = os.WriteFile(triagePath, []byte(content), 0644)

		fmt.Println("\n======================================================================")
		fmt.Println(" [AI Triage Summary]")
		fmt.Println("======================================================================")
		fmt.Println(summary)
		fmt.Println("======================================================================")
		fmt.Printf("\n[+] Triage report saved to: %s\n", triagePath)

		return nil
	},
}

func init() {
	triageCmd.Flags().StringVar(&triageRunID, "run-id", "", "Run ID to triage")
	triageCmd.Flags().StringVar(&triageModel, "model", "", "AI model name (e.g. musespark-1.3, overrides config/env)")
	triageCmd.Flags().StringVar(&triageBaseURL, "base-url", "", "AI endpoint base URL (e.g. https://opencode.ai/zen/v1)")
	triageCmd.Flags().StringVar(&triageAPIKey, "api-key", "", "AI API key / token (overrides config/env)")
	rootCmd.AddCommand(triageCmd)
}
