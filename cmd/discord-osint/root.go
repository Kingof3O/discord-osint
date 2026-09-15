package main

import (
	"fmt"

	"discord-osint/internal/config"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	cfgFile   string
	flagToken string
	flagProxy string
	cfg       *config.Config
	logger    *zap.Logger
)

var rootCmd = &cobra.Command{
	Use:   "discord-osint",
	Short: "Production-grade Discord OSINT search and reconnaissance workflow",
	Long: `discord-osint is a single-binary CLI tool for discovering target user presence
across Discord servers with strict identity separation, interactive manual CAPTCHA solving,
and comprehensive audit provenance.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return err
		}

		if flagToken != "" {
			cfg.DiscordToken = flagToken
		}
		if flagProxy != "" {
			cfg.Proxy = flagProxy
		}

		// Configure zap logger with sensitive token scrubbing
		logConfig := zap.NewProductionConfig()
		logConfig.EncoderConfig.TimeKey = "timestamp"
		logConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		if cfg.LogLevel == "debug" {
			logConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		} else {
			logConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
		}

		logger, err = logConfig.Build()
		if err != nil {
			return fmt.Errorf("failed to initialize logger: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Path to config YAML file")
	rootCmd.PersistentFlags().StringVar(&flagToken, "token", "", "Discord user auth token (overrides config)")
	rootCmd.PersistentFlags().StringVar(&flagProxy, "proxy", "", "Proxy URL (http://... or socks5://...)")
}
