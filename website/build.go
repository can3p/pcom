package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed assets/style.css
var styleCSS []byte

// Config is what the command line supplies.
type Config struct {
	Repo string // repository root
	Out  string // output directory
}

// view is the data every template renders against.
type view struct {
	Site *Site
	Page *Page
	Nav  []NavSection
}

func (v view) Rel(to string) string { return relPath(v.Page.Path, to) }

func (v view) Active(p string) bool { return v.Page.Path == p }

func (v view) SourceURL() string {
	return fmt.Sprintf("%s/blob/%s/%s", repoURL, repoRef, v.Page.Src)
}

func (p *Page) IsLanding() bool { return p.Kind == KindLanding }

// LandingView is the landing page: rendered slices of files that already
// exist, plus one card per guide page.
type LandingView struct {
	Lede       template.HTML
	Quickstart template.HTML
	Cards      []GuideCard
}

// GuideCard is a guide page's title, first paragraph and first image.
type GuideCard struct {
	Title, Path string
	Intro       template.HTML
	Image, Alt  string // Image is empty for a page without one
}

// Build generates the whole site into cfg.Out and returns the site model, so
// tests can assert against the same structure the pages were written from.
func Build(cfg Config) (*Site, error) {
	site := newSite(cfg.Repo)

	tmpl, err := template.New("site").ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	// Bodies first: the landing page renders slices of files that other pages
	// also render, and every link is rewritten through the same map.
	for _, p := range site.Pages {
		switch p.Kind {
		case KindMarkdown:
			doc, err := site.md.Parse(p.Src, p.Path)
			if err != nil {
				return nil, err
			}
			body, err := doc.HTML()
			if err != nil {
				return nil, err
			}
			p.Body = body
			p.Heading = doc.Heading()
			if h := doc.Headings(); len(h) > 2 {
				p.TOC = h
			}
		case KindLanding:
			landing, err := site.landing()
			if err != nil {
				return nil, err
			}
			var buf bytes.Buffer
			if err := tmpl.ExecuteTemplate(&buf, "landing", landing); err != nil {
				return nil, err
			}
			p.Body = template.HTML(buf.String()) //nolint:gosec // rendered by our own template
		}
	}
	if len(site.problems) > 0 {
		sort.Strings(site.problems)
		return nil, fmt.Errorf("%s", strings.Join(dedupe(site.problems), "; "))
	}

	if err := os.MkdirAll(cfg.Out, 0o755); err != nil {
		return nil, err
	}
	for _, p := range site.Pages {
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "layout", view{Site: site, Page: p, Nav: site.Nav}); err != nil {
			return nil, fmt.Errorf("%s: %w", p.Path, err)
		}
		if err := writeFile(filepath.Join(cfg.Out, filepath.FromSlash(p.Path)), buf.Bytes()); err != nil {
			return nil, err
		}
	}
	for img := range site.images {
		if err := copyFile(filepath.Join(cfg.Repo, filepath.FromSlash(img)),
			filepath.Join(cfg.Out, filepath.FromSlash(img))); err != nil {
			return nil, err
		}
	}
	if err := writeFile(filepath.Join(cfg.Out, "style.css"), styleCSS); err != nil {
		return nil, err
	}
	// GitHub Pages runs Jekyll over the artifact unless told not to.
	if err := writeFile(filepath.Join(cfg.Out, ".nojekyll"), nil); err != nil {
		return nil, err
	}
	return site, nil
}

func dedupe(sorted []string) []string {
	var out []string
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}

func writeFile(name string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	return os.WriteFile(name, data, 0o644)
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return writeFile(to, data)
}

// landing assembles the one page with any words of its own: none. The lede and
// the quick start are slices of README.md, and each card is a slice of a guide
// page.
func (s *Site) landing() (*LandingView, error) {
	readme, err := s.md.Parse("README.md", "index.html")
	if err != nil {
		return nil, err
	}
	v := &LandingView{}
	if v.Lede, err = readme.Preamble(); err != nil {
		return nil, err
	}
	if v.Quickstart, err = readme.Section(quickstartSection); err != nil {
		return nil, err
	}
	for _, p := range s.guidePages() {
		doc, err := s.md.Parse(p.Src, "index.html")
		if err != nil {
			return nil, err
		}
		card := GuideCard{Title: doc.Heading(), Path: p.Path}
		if card.Title == "" {
			return nil, fmt.Errorf("%s: no H1 to title its card", p.Src)
		}
		if card.Intro, err = doc.FirstParagraph(); err != nil {
			return nil, err
		}
		card.Image, card.Alt, _ = doc.FirstImage()
		v.Cards = append(v.Cards, card)
	}
	return v, nil
}

// quickstartSection is the README section the landing page shows as the quick
// start.
const quickstartSection = "Local development"
