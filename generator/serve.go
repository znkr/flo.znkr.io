package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/markst"
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

		// One cache for the life of the process. A build declares what the site
		// is made of and computes none of it; a page is rendered when it is
		// first requested, and held until something it depends on changes.
		cache := build.NewCache(cacheSize)

		start := time.Now()
		s, err := reload(cmd.Context(), cache, dir)
		if err != nil {
			return fmt.Errorf("loading site: %v", err)
		}
		cache.Stats()
		log.Printf("Site loaded (%v)", time.Since(start))

		// Start serving.
		const addr = "localhost:8080"
		server, err := server.Run(addr, cache, s)
		if err != nil {
			return err
		}
		defer server.Shutdown(context.Background())
		log.Printf("Now serving at %s, press Ctrl-C to shut down", addr)

		// Setup file watcher to trigger reloading of the site should anything change on disk.
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("starting watcher: %v", err)
		}
		defer watcher.Close()
		for _, subdir := range source.Dirs {
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
		const debounceDelay = 100 * time.Millisecond
		var debounce <-chan time.Time

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
					log.Printf("Added watch directory: %v", wd)
				}

				debounce = time.After(debounceDelay)

			case <-debounce:
				debounce = nil

				start := time.Now()
				// Discard what serving the pages did since the last reload, so
				// that the counts below cover only this one.
				cache.Stats()
				s, err := reload(cmd.Context(), cache, dir)
				if err != nil {
					log.Printf("failed to update site: %v", err)
					continue
				}
				st := cache.Stats()
				server.ReplaceSite(s)
				log.Printf("Site reloaded (%v, %d documents recompiled, %d of %d steps reused)",
					time.Since(start), st.Kinds["doc"].Misses, st.Hits, st.Hits+st.Misses)

			case err := <-server.Error():
				return fmt.Errorf("serving: %v", err)

			case err := <-watcher.Errors:
				return fmt.Errorf("watching: %v", err)

			case <-sigint:
				fmt.Print("\r") // remove Ctrl-C output characters
				log.Printf("Received Ctrl-C, shutting down")
				return nil
			}
		}
	},
}

// reload scans dir, and plans the site, and compiles all documents.
//
// Planning alone computes nothing, so the site would be served without a line
// of it having been read. The compile is forced because a document that no
// longer compiles, or that warns, belongs in the terminal now rather than in
// whichever request first reaches it. Forcing the metadata renders the summary
// with it, and highlights the fragments the summary holds. The body is what
// waits for a request.
//
// The warnings are reported in full every reload, including the ones from
// documents this reload did not have to compile, so that a warning stays on
// screen until the document it is about is fixed.
func reload(ctx context.Context, c *build.Cache, dir string) (*site.Site, error) {
	tree, err := source.Scan(dir)
	if err != nil {
		return nil, fmt.Errorf("scanning site: %v", err)
	}
	s, diags, err := plan(tree)
	if err != nil {
		return nil, err
	}
	for _, d := range s.Docs() {
		if _, err := d.Meta.Get(ctx, c); err != nil {
			return nil, err
		}
	}
	ds, err := diags.Get(ctx, c)
	if err != nil {
		return nil, err
	}
	markst.Report(ds)
	return s, nil
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
