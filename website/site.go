package main

import (
	"html/template"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// repoURL is where a file the site does not publish is linked instead.
const (
	repoURL = "https://github.com/can3p/pcom"
	repoRef = "master"
)

// Kind distinguishes how a page's body is produced.
type Kind int

const (
	KindLanding Kind = iota
	KindMarkdown
)

// GroupGuide is the sidebar's one group. The site publishes the user guide
// only; developer docs are linked to on GitHub.
const GroupGuide = "Guide"

// Page is one HTML file on the site.
type Page struct {
	Path    string // site-relative output path, e.g. "docs/guide/overview.html"
	Title   string // <title> and nav label
	Heading string // the source document's own H1, when it has one
	Group   string // sidebar group
	Kind    Kind
	Src     string // repo-relative source file ("" for the landing page)
	Body    template.HTML
	TOC     []Head
}

type Head struct {
	ID, Text string
}

// NavItem is one entry in the sidebar.
type NavItem struct {
	Title string
	Path  string
}

type NavSection struct {
	Title string
	Items []NavItem
}

// Site holds everything the templates need, plus the maps that make link
// rewriting possible: repo path -> site path, and site path -> page.
type Site struct {
	Repo  string
	Pages []*Page
	Nav   []NavSection

	byPath     map[string]*Page
	repoToSite map[string]string

	// unpublished records repo-relative targets that no page renders, and the
	// source files that linked to them. The coverage test asserts the set.
	unpublished map[string][]string

	// images are repo-relative image files referenced by rendered pages; the
	// build copies them. problems collects what the renderer could not return
	// as an error (a missing image).
	images   map[string]bool
	problems []string

	md *Renderer
}

func (s *Site) Page(sitePath string) *Page { return s.byPath[sitePath] }

// SitePathFor maps a repo-relative path to the page that renders it.
func (s *Site) SitePathFor(repoPath string) (string, bool) {
	p, ok := s.repoToSite[repoPath]
	return p, ok
}

// Unpublished returns repo paths linked from the documentation that the site
// does not render, with the files that linked to them.
func (s *Site) Unpublished() map[string][]string { return s.unpublished }

// docPages are the repository documents that get a page, with the short label
// the sidebar uses for each. Navigation labels only: every word of the page
// itself comes from the file. One line per page; a feature wave's docs task
// adds its guide page here.
var docPages = []struct {
	Group, Src, Out, Nav string
}{
	{GroupGuide, "docs/guide/overview.md", "docs/guide/overview.html", "Overview"},
	{GroupGuide, "docs/guide/connections.md", "docs/guide/connections.html", "Connections"},
	{GroupGuide, "docs/guide/writing.md", "docs/guide/writing.html", "Writing"},
	{GroupGuide, "docs/guide/feed.md", "docs/guide/feed.html", "Feed and RSS"},
	{GroupGuide, "docs/guide/comments.md", "docs/guide/comments.html", "Comments"},
	{GroupGuide, "docs/guide/settings.md", "docs/guide/settings.html", "Settings"},
	{GroupGuide, "docs/guide/api.md", "docs/guide/api.html", "API and blg"},
	{GroupGuide, "docs/guide/self-hosting.md", "docs/guide/self-hosting.html", "Self-hosting"},
}

// newSite builds the page list and the repo->site path map.
func newSite(repo string) *Site {
	s := &Site{
		Repo:        repo,
		byPath:      map[string]*Page{},
		repoToSite:  map[string]string{},
		unpublished: map[string][]string{},
		images:      map[string]bool{},
	}
	s.md = NewRenderer(s)

	add := func(p *Page) {
		s.Pages = append(s.Pages, p)
		s.byPath[p.Path] = p
		if p.Src != "" {
			s.repoToSite[p.Src] = p.Path
		}
	}
	add(&Page{Path: "index.html", Title: "pcom", Kind: KindLanding})
	for _, d := range docPages {
		add(&Page{Path: d.Out, Title: d.Nav, Group: d.Group, Kind: KindMarkdown, Src: d.Src})
	}
	s.buildNav()
	return s
}

func (s *Site) buildNav() {
	for _, g := range []string{GroupGuide} {
		sec := NavSection{Title: g}
		for _, p := range s.Pages {
			if p.Group == g {
				sec.Items = append(sec.Items, NavItem{Title: p.Title, Path: p.Path})
			}
		}
		if len(sec.Items) > 0 {
			s.Nav = append(s.Nav, sec)
		}
	}
}

// guidePages are the pages of the Guide group, in sidebar order.
func (s *Site) guidePages() []*Page {
	var out []*Page
	for _, p := range s.Pages {
		if p.Group == GroupGuide {
			out = append(out, p)
		}
	}
	return out
}

func (s *Site) fileExists(repoPath string) bool {
	fi, err := os.Stat(filepath.Join(s.Repo, filepath.FromSlash(repoPath)))
	return err == nil && !fi.IsDir()
}

// relPath renders target as a link relative to the page at from. Slash-based
// throughout: these are URLs, not filesystem paths.
func relPath(from, to string) string {
	fromDir := path.Dir(from)
	var f []string
	if fromDir != "." {
		f = strings.Split(fromDir, "/")
	}
	t := strings.Split(to, "/")
	i := 0
	for i < len(f) && i < len(t)-1 && f[i] == t[i] {
		i++
	}
	var parts []string
	for j := i; j < len(f); j++ {
		parts = append(parts, "..")
	}
	parts = append(parts, t[i:]...)
	return strings.Join(parts, "/")
}
