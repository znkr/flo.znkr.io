package main

import (
	"fmt"
	"os"

	"flo.znkr.io/generator/pack"
	"github.com/spf13/cobra"
)

var packCmd = &cobra.Command{
	Use:   "pack",
	Short: "Packs the site into .tar file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("determining workdir: %v", err)
		}
		s, err := load(dir)
		if err != nil {
			return fmt.Errorf("loading site: %v", err)
		}
		return pack.Pack(args[0], s)
	},
}
