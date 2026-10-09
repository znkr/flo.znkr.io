package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/markst"
	"flo.znkr.io/generator/plan"
	"flo.znkr.io/generator/server"
	"flo.znkr.io/generator/site"
	"flo.znkr.io/generator/source"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve site without actually generating any file",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("determining workdir: %v", err)
		}
		var opts plan.Options
		opts.Drafts, err = cmd.Flags().GetBool("drafts")
		if err != nil {
			return err
		}

		// One cache for the life of the process. A build declares what the site
		// is made of and computes none of it; a page is rendered when it is
		// first requested, and held until something it depends on changes.
		cache := build.NewCache(cacheSize)

		start := time.Now()
		s, report, loadErr := reload(cmd.Context(), cache, dir, opts)
		if loadErr != nil {
			// An empty site is served, so that the browser shows the failure
			// and the next change to the files is picked up.
			slog.Error("failed to load site", "err", loadErr)
			s, _ = site.New(nil)
		} else {
			slog.Info("site loaded", "took", time.Since(start))
		}
		cache.Stats()

		// Start serving.
		const addr = "localhost:8080"
		server, err := server.Run(addr, cache, s, report)
		if err != nil {
			return err
		}
		defer server.Close()
		if loadErr != nil {
			server.ReportFailure(loadErr)
		}
		slog.Info("serving, press Ctrl-C to shut down", "url", "http://"+addr)

		// Setup file watcher to trigger reloading of the site should anything change on disk.
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("starting watcher: %v", err)
		}
		defer watcher.Close()
		// The generator's own source is watched too: a change to it rebuilds
		// the generator and restarts serve with it.
		code := filepath.Join(dir, "generator")
		for _, subdir := range append(slices.Clone(source.Dirs), "generator") {
			if err := watchDir(watcher, filepath.Join(dir, subdir)); err != nil {
				return fmt.Errorf("starting watch: %v", err)
			}
		}

		// Setup signals to react to Ctrl-C.
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)

		// A single save usually produces several events. Rebuilding once per
		// event would be wasteful and, worse, would refresh the browser
		// several times, so events are coalesced.
		const debounceDelay = 30 * time.Millisecond
		var debounce, restart <-chan time.Time

		for {
			select {
			case event := <-watcher.Events:
				// Absolutely no need to react to chmod.
				if event.Has(fsnotify.Chmod) {
					continue
				}

				// Update watch list should new directories be added or removed.
				if stat, err := os.Stat(event.Name); err == nil && event.Has(fsnotify.Create) && stat.IsDir() {
					if err := watchDir(watcher, event.Name); err != nil {
						return fmt.Errorf("adding watch: %v", err)
					}
					wd, _ := filepath.Rel(dir, event.Name)
					slog.Info("watching directory", "dir", wd)
				}

				switch rel, err := filepath.Rel(code, event.Name); {
				case err != nil || strings.HasPrefix(rel, ".."):
					debounce = time.After(debounceDelay)
				case isGeneratorSource(rel):
					restart = time.After(debounceDelay)
				}

			case <-restart:
				restart = nil

				start := time.Now()
				bin, err := buildGenerator(cmd.Context(), dir)
				if err != nil {
					// The running generator keeps serving, and the next change
					// to its source tries again.
					// The error holds the compiler's output. A message is
					// printed line by line, an attribute quoted.
					slog.Error(err.Error())
					server.ReportFailure(err)
					continue
				}
				slog.Info("generator rebuilt, restarting", "took", time.Since(start))
				// Exec does not return on success, so nothing deferred runs. The
				// server is closed here, so that the new process can listen on
				// the same address and every browser reconnects to it.
				server.Close()
				watcher.Close()
				if err := syscall.Exec(bin, os.Args, os.Environ()); err != nil {
					return fmt.Errorf("restarting: %v", err)
				}

			case <-debounce:
				debounce = nil

				start := time.Now()
				// Discard what serving the pages did since the last reload, so
				// that the counts below cover only this one.
				cache.Stats()
				s, report, err := reload(cmd.Context(), cache, dir, opts)
				if err != nil {
					slog.Error("failed to reload site", "err", err)
					server.ReportFailure(err)
					continue
				}
				st := cache.Stats()
				server.ReplaceSite(s, report)
				slog.Info("site reloaded", "took", time.Since(start),
					"recompiled", st.Kinds["doc"].Misses, "reused", fmt.Sprintf("%d/%d", st.Hits, st.Hits+st.Misses))

			case err := <-server.Error():
				return fmt.Errorf("serving: %v", err)

			case err := <-watcher.Errors:
				return fmt.Errorf("watching: %v", err)

			case <-sigint:
				fmt.Print("\r") // remove Ctrl-C output characters
				slog.Info("shutting down")
				return nil
			}
		}
	},
}

func init() {
	serveCmd.Flags().Bool("drafts", false, "list drafts on the index and in the feed")
}

// reload scans dir, plans the site with opts, and compiles all documents. It fails only
// if the site cannot be planned. A document that fails is reported and the
// site is served regardless; its page shows the failure when it is requested.
//
// Planning alone computes nothing, so the site would be served without a line
// of it having been read. The compile is forced because a document that does
// not compile, or that warns, belongs in the terminal now rather than in
// whichever request first reaches it. Forcing the metadata renders the summary
// with it, and highlights the fragments the summary holds. The body is what
// waits for a request.
//
// The diagnostics are reported in full every reload, including the ones from
// documents this reload did not have to compile, so that a diagnostic stays on
// screen until the document it is about is fixed. The returned report holds
// the warnings and every failure, formatted for the browser.
func reload(ctx context.Context, c *build.Cache, dir string, opts plan.Options) (*site.Site, string, error) {
	tree, err := source.Scan(dir)
	if err != nil {
		return nil, "", fmt.Errorf("scanning site: %v", err)
	}
	s, diags, err := opts.Plan(tree)
	if err != nil {
		return nil, "", err
	}
	var failures []string
	for _, d := range s.Docs() {
		if _, err := d.Meta.Get(ctx, c); err != nil {
			failures = append(failures, err.Error())
		}
	}
	ds, err := diags.Get(ctx, c)
	if err != nil {
		failures = append(failures, err.Error())
	}
	markst.Report(ds)
	// A failure in a step many documents share, such as a library compile
	// that could not run, is the failure of each of them.
	slices.Sort(failures)
	failures = slices.Compact(failures)
	for _, f := range failures {
		slog.Error(f)
	}
	// An error in a document is shown by the document's own page, so the
	// browser is told of the warnings and of the failures.
	var report []string
	if ws := markst.OfSeverity(ds, markst.Warning); len(ws) > 0 {
		report = append(report, markst.Format(ws))
	}
	report = append(report, failures...)
	return s, strings.Join(report, "\n"), nil
}

// isGeneratorSource reports whether the file at rel, relative to the generator's
// directory, is part of the generator binary.
func isGeneratorSource(rel string) bool {
	if filepath.Ext(rel) != ".go" || strings.HasSuffix(rel, "_test.go") {
		return false
	}
	return !slices.Contains(strings.Split(filepath.ToSlash(rel), "/"), "testdata")
}

// buildGenerator builds the generator from its source in dir and returns the
// path of the binary. The error of a failed build holds the compiler's output.
func buildGenerator(ctx context.Context, dir string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(cache, "flo.znkr.io", "serve")
	if err := os.MkdirAll(filepath.Dir(bin), 0755); err != nil {
		return "", err
	}
	// The binary is built under another name and renamed into place, because
	// the running process can have been started from bin, and macOS kills a
	// process whose binary is written to.
	tmp := bin + ".new"
	cmd := exec.CommandContext(ctx, "go", "build", "-o", tmp, "./generator")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building generator: %v\n%s", err, out)
	}
	if err := os.Rename(tmp, bin); err != nil {
		return "", err
	}
	return bin, nil
}

func watchDir(watcher *fsnotify.Watcher, dir string) error {
	walkfn := func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if err := watcher.Add(path); err != nil {
				return err
			}
		}
		return err
	}
	if err := filepath.WalkDir(dir, walkfn); err != nil {
		return err
	}
	return nil
}
