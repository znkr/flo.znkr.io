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

	"flo.znkr.io/generator/server"
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
		s, err := load(dir)
		if err != nil {
			return fmt.Errorf("loading site: %v", err)
		}

		// Start serving.
		const addr = "localhost:8080"
		server, err := server.Run(addr, s)
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
		for _, subdir := range []string{"site", "templates"} {
			if err := watchDir(watcher, filepath.Join(dir, subdir)); err != nil {
				return fmt.Errorf("starting watch: %v", err)
			}
		}

		// Setup signals to react to Ctrl-C.
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)

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

				// Reload site. This is more than fast enough for now, so now caching or anything
				// is necessary here.
				start := time.Now()
				s, err := load(dir)
				if err != nil {
					log.Printf("failed to update site: %v", err)
					continue
				}
				server.ReplaceSite(s)
				d := time.Since(start)
				log.Printf("Site reloaded (%v)", d)

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
