// Command website renders the pcom documentation as a static site.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.Repo, "repo", "..", "path to the pcom repository root")
	flag.StringVar(&cfg.Out, "out", "../site", "directory to write the site into")
	flag.Parse()

	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected argument %q\n", flag.Arg(0))
		os.Exit(2)
	}

	site, err := Build(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
	abs, _ := filepath.Abs(cfg.Out)
	fmt.Printf("wrote %d pages to %s\n", len(site.Pages), abs)
	var paths []string
	for repoPath := range site.Unpublished() {
		paths = append(paths, repoPath)
	}
	sort.Strings(paths)
	for _, repoPath := range paths {
		fmt.Printf("  not published, linked to GitHub: %s (from %v)\n", repoPath, site.Unpublished()[repoPath])
	}
}
