package main

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

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
