package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/logs"
	"flo.znkr.io/generator/markst"
	"flo.znkr.io/generator/plan"
	"flo.znkr.io/generator/site"
	"flo.znkr.io/generator/source"
	"github.com/spf13/cobra"
)

// cacheSize is how many computed values are held.
//
// It counts items, which is a proxy for the two costs that matter: what a value
// takes to hold and what it takes to compute again. One build of this site
// leaves 151 values behind, about 700KB of them bytes, and the spread between
// the largest and the average is around twentyfold. So this is a dozen or so
// builds of history, which is as far back as an edit is worth reusing.
const cacheSize = 2048

// loadDir scans dir, plans the site it holds, and returns it with a cache to
// compute it against. It fails if compiling the site produces any diagnostic,
// a warning included, and reports them.
func loadDir(ctx context.Context, dir string) (*build.Cache, *site.Site, error) {
	tree, err := source.Scan(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("scanning %s: %v", dir, err)
	}
	s, diags, err := plan.Plan(tree)
	if err != nil {
		return nil, nil, err
	}
	c := build.NewCache(cacheSize)
	ds, err := diags.Get(ctx, c)
	if err != nil {
		return nil, nil, err
	}
	if len(ds) > 0 {
		markst.Report(ds)
		return nil, nil, fmt.Errorf("%d diagnostics", len(ds))
	}
	return c, s, nil
}

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
