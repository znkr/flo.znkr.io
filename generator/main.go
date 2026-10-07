package main

import (
	"log/slog"
	"os"

	"flo.znkr.io/generator/logs"
	"github.com/spf13/cobra"
)

func main() {
	// The default logger also takes what the log package writes, net/http's
	// messages included.
	slog.SetDefault(slog.New(logs.NewHandler(os.Stderr, slog.LevelInfo, logs.Color(os.Stderr))))

	rootCmd := &cobra.Command{
		Use:          "site [command]",
		Short:        "Static side generator for znkr.io",
		SilenceUsage: true,
	}

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(packCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
