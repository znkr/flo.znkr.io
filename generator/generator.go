package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"flo.znkr.io/generator/pack"
	"flo.znkr.io/generator/server"
	"flo.znkr.io/generator/site"
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

		s, err := site.Load(dir)
		if err != nil {
			return fmt.Errorf("loading site: %v", err)
		}

		// Start serving.
		const addr = "localhost:8080"
		server := server.New(addr, s)
		go server.Start()
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
		{
			wl := watcher.WatchList()
			for i := range wl {
				wl[i], _ = filepath.Rel(dir, wl[i])
			}
			slices.Sort(wl)
			log.Printf("Watching:\n    %v", strings.Join(wl, "\n    "))
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
				switch stat, err := os.Stat(event.Name); {
				case os.IsNotExist(err) && (event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)):
					if slices.Contains(watcher.WatchList(), event.Name) {
						watcher.Remove(event.Name)
						wd, _ := filepath.Rel(dir, event.Name)
						log.Printf("Removed watch directory: %v", wd)
					}
				case err == nil && event.Has(fsnotify.Create) && stat.IsDir():
					if err := watchDir(watcher, event.Name); err != nil {
						return fmt.Errorf("adding watch: %v", err)
					}
					wd, _ := filepath.Rel(dir, event.Name)
					log.Printf("Added watch directory: %v", wd)
				case err != nil:
					return fmt.Errorf("watching site: %v", err)
				}

				// Reload site. This is more than fast enough for now, so now caching or anything
				// is necessary here.
				start := time.Now()
				s, err := site.Load(dir)
				if err != nil {
					log.Printf("failed to update site: %v", err)
					continue
				}
				server.ReplaceSite(s)
				d := time.Since(start)
				log.Printf("Site reloaded (%v)", d)
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

var packCmd = &cobra.Command{
	Use:   "pack",
	Short: "Packs the site into .tar file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("determining workdir: %v", err)
		}
		s, err := site.Load(dir)
		if err != nil {
			return fmt.Errorf("loading site: %v", err)
		}
		return pack.Pack(args[0], s)
	},
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	rootCmd := &cobra.Command{
		Use:   "site [command]",
		Short: "Static side generator for znkr.io",
	}

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(packCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
