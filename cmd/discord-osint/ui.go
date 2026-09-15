package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"discord-osint/internal/store"
	"discord-osint/internal/ui"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	uiHost string
	uiPort int
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch the embedded web dashboard for real-time investigation monitoring",
	Long: `Starts a local, self-contained HTTP web server providing an interactive
dark-themed dashboard to inspect runs, server scans, confirmed matches, and evidence.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := cfg.DatabasePath
		if dbPath == "" {
			dbPath = "run.sqlite"
		}
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer s.Close()

		addr := fmt.Sprintf("%s:%d", uiHost, uiPort)
		server := ui.NewServer(ui.ServerOptions{
			Addr:   addr,
			Store:  s,
			Logger: logger,
		})

		// Graceful shutdown on SIGINT/SIGTERM
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

		go func() {
			<-stop
			fmt.Println("\n[*] Shutting down web dashboard...")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(ctx); err != nil {
				logger.Error("Error during server shutdown", zap.Error(err))
			}
		}()

		fmt.Printf("\n=======================================================\n")
		fmt.Printf("   DISCORD OSINT WEB DASHBOARD\n")
		fmt.Printf("   Local URL: http://%s\n", addr)
		fmt.Printf("   Press Ctrl+C to terminate\n")
		fmt.Printf("=======================================================\n\n")

		return server.Start()
	},
}

func init() {
	uiCmd.Flags().StringVar(&uiHost, "host", "127.0.0.1", "Host interface to bind web dashboard")
	uiCmd.Flags().IntVarP(&uiPort, "port", "p", 3000, "Port for web dashboard")
	rootCmd.AddCommand(uiCmd)
}
