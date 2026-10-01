package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestSite is the production site: the real docPages map, over repo.
func newTestSite(t *testing.T, repo string) *Site {
	t.Helper()
	return newSite(repo)
}

// Link rewriting is the fiddly part of the generator: the documentation is
// full of paths that are correct relative to the file they live in and wrong
// everywhere else.
func TestResolveLink(t *testing.T) {
	s := newTestSite(t, "..")
	const gh = "https://github.com/can3p/pcom"

	cases := []struct {
		name          string
		src, page, in string
		want          string
	}{
		{"external link is untouched", "README.md", "readme.html",
			"https://github.com/can3p/blg", "https://github.com/can3p/blg"},
		{"scheme-relative is untouched", "README.md", "readme.html", "//example.com/x", "//example.com/x"},
		{"mailto is untouched", "README.md", "readme.html", "mailto:a@example.com", "mailto:a@example.com"},
		{"in-page anchor is untouched", "README.md", "readme.html", "#ports", "#ports"},
		{"empty is untouched", "README.md", "readme.html", "", ""},

		{"sibling document from the root", "README.md", "readme.html",
			"docs/architecture.md", "docs/architecture.html"},
		{"a guide page from the landing page", "README.md", "index.html",
			"docs/guide/overview.md", "docs/guide/overview.html"},
		{"up and across from a guide page", "docs/guide/overview.md", "docs/guide/overview.html",
			"../architecture.md", "../architecture.html"},
		{"a fragment survives the rewrite", "README.md", "readme.html",
			"docs/architecture.md#layers", "docs/architecture.html#layers"},
		{"a query string survives the rewrite", "README.md", "readme.html",
			"docs/architecture.md?x=1", "docs/architecture.html?x=1"},
		{"an archived document keeps its new home", "docs/architecture.md", "docs/architecture.html",
			"archive/history.md", "history.html"},

		{"a file the site does not publish goes to GitHub", "README.md", "readme.html",
			"cmd/web/client/articles/why.md", gh + "/blob/master/cmd/web/client/articles/why.md"},
		{"a directory the site does not publish goes to GitHub's tree view",
			"docs/architecture.md", "docs/architecture.html",
			"../pkg", gh + "/tree/master/pkg"},
		{"an unpublished target keeps its fragment", "docs/architecture.md", "docs/architecture.html",
			"../Makefile#L10", gh + "/blob/master/Makefile#L10"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := s.ResolveLink(c.src, c.page, c.in); got != c.want {
				t.Errorf("ResolveLink(%q, %q, %q) = %q, want %q", c.src, c.page, c.in, got, c.want)
			}
		})
	}

	if len(s.problems) != 0 {
		t.Errorf("unexpected problems: %v", s.problems)
	}
	if from := s.Unpublished()["Makefile"]; len(from) != 1 || from[0] != "docs/architecture.md" {
		t.Errorf("unpublished links not recorded: %v", s.Unpublished())
	}
}

// A link that climbs above the repository root is a build problem, not
// something to clamp to the root.
func TestResolveLinkAboveRootIsAProblem(t *testing.T) {
	s := newTestSite(t, "..")
	if got := s.ResolveLink("README.md", "readme.html", "../outside.md"); got != "../outside.md" {
		t.Errorf("got %q", got)
	}
	if got := s.ResolveLink("docs/running.md", "docs/running.html", "../../outside.md"); got != "../../outside.md" {
		t.Errorf("got %q", got)
	}
	if len(s.problems) != 2 || !strings.Contains(s.problems[0], "climbs above") {
		t.Errorf("problems = %v", s.problems)
	}
	if len(s.Unpublished()) != 0 {
		t.Errorf("a link above the root must not be recorded as unpublished: %v", s.Unpublished())
	}
}

func TestRelPath(t *testing.T) {
	cases := []struct{ from, to, want string }{
		{"index.html", "style.css", "style.css"},
		{"index.html", "docs/running.html", "docs/running.html"},
		{"docs/running.html", "style.css", "../style.css"},
		{"docs/guide/overview.html", "style.css", "../../style.css"},
		{"docs/guide/overview.html", "docs/running.html", "../running.html"},
		{"docs/guide/overview.html", "docs/guide/feed.html", "feed.html"},
		{"docs/running.html", "readme.html", "../readme.html"},
	}
	for _, c := range cases {
		if got := relPath(c.from, c.to); got != c.want {
			t.Errorf("relPath(%q, %q) = %q, want %q", c.from, c.to, got, c.want)
		}
	}
}

// The rewriting has to survive the round trip through goldmark, including
// links inside tables and reference-style links.
func TestMarkdownRendering(t *testing.T) {
	repo := t.TempDir()
	md := "# Overview\n\n" +
		"See [the readme](../../README.md) and [the architecture](../architecture.md).\n\n" +
		"| Flag | What |\n|---|---|\n| `--port` | see [arch][c] |\n\n" +
		"[c]: ../architecture.md#layers\n\n" +
		"## What it is\n\nA network.\n\n" +
		"## What it's for\n\n```bash\ncurl -s localhost\n```\n"
	src := "docs/guide/overview.md"
	writeFixture(t, repo, src, md)

	s := newTestSite(t, repo)
	doc, err := s.md.Parse(src, "docs/guide/overview.html")
	if err != nil {
		t.Fatal(err)
	}
	html, err := doc.HTML()
	if err != nil {
		t.Fatal(err)
	}
	got := string(html)

	for _, want := range []string{
		`href="../../readme.html"`,
		`href="../architecture.html"`,
		`href="../architecture.html#layers"`, // a reference-style link with a fragment
		"<table>", "<th>Flag</th>",           // GFM tables are enabled
		`<code class="language-bash">`, // fenced code keeps its language
		`<h2 id="what-it-is">`,         // headings get anchors
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered HTML does not contain %q:\n%s", want, got)
		}
	}

	if h := doc.Heading(); h != "Overview" {
		t.Errorf("Heading() = %q", h)
	}
	heads := doc.Headings()
	if len(heads) != 2 || heads[0].Text != "What it is" || heads[0].ID != "what-it-is" {
		t.Errorf("Headings() = %+v", heads)
	}

	section, err := doc.Section("What it is")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(section), "A network.") || strings.Contains(string(section), "curl") {
		t.Errorf("Section() spilled past its own heading: %s", section)
	}
	if _, err := doc.Section("No Such Section"); err == nil {
		t.Error("Section() of a missing heading should fail the build")
	}
	pre, err := doc.Preamble()
	if err != nil || !strings.Contains(string(pre), "the readme") || strings.Contains(string(pre), "A network") {
		t.Errorf("Preamble() = %q, %v", pre, err)
	}
	p, err := doc.FirstParagraph()
	if err != nil || !strings.Contains(string(p), "See ") {
		t.Errorf("FirstParagraph() = %q, %v", p, err)
	}
	if _, _, ok := doc.FirstImage(); ok {
		t.Error("FirstImage() found an image in a document without one")
	}
}

func TestFirstParagraphMissing(t *testing.T) {
	repo := t.TempDir()
	writeFixture(t, repo, "docs/x.md", "# Only a title\n")
	s := newTestSite(t, repo)
	doc, err := s.md.Parse("docs/x.md", "index.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.FirstParagraph(); err == nil {
		t.Error("a page without a paragraph should fail its card")
	}
}

func TestImageLinks(t *testing.T) {
	repo := t.TempDir()
	writeFixture(t, repo, "docs/guide/screenshots/a.png", "png")
	s := newTestSite(t, repo)
	if got := s.ResolveImage("docs/guide/overview.md", "docs/guide/overview.html", "screenshots/a.png"); len(s.problems) != 0 || got != "screenshots/a.png" {
		t.Errorf("got %q", got)
	}
	if got := s.ResolveImage("docs/guide/overview.md", "index.html", "screenshots/a.png"); got != "docs/guide/screenshots/a.png" {
		t.Errorf("got %q", got)
	}
	if got := s.ResolveImage("README.md", "readme.html", "https://example.com/a.png"); got != "https://example.com/a.png" {
		t.Errorf("got %q", got)
	}
	if got := s.ResolveImage("README.md", "readme.html", "../../etc/passwd.png"); got != "../../etc/passwd.png" || len(s.problems) != 1 {
		t.Errorf("an image outside the repository should be a problem: %q %v", got, s.problems)
	}
}

// Raw HTML must never reach a page: the documents in this repository contain
// none, and the renderer is configured so that any that appeared could not.
func TestRawHTMLIsNotPassedThrough(t *testing.T) {
	const body = "<script>alert(1)</script>\n\nplain\n\nan <b onclick=x>inline</b> case\n"
	check := func(t *testing.T, html string) {
		t.Helper()
		for _, bad := range []string{"<script>", "<b onclick", "alert(1)</script>"} {
			if strings.Contains(html, bad) {
				t.Errorf("raw HTML %q reached the page: %s", bad, html)
			}
		}
		for _, want := range []string{"plain", "inline", "case"} {
			if !strings.Contains(html, want) {
				t.Errorf("surrounding text %q was dropped: %s", want, html)
			}
		}
	}

	repo := t.TempDir()
	writeFixture(t, repo, "docs/evil.md", body)
	s := newTestSite(t, repo)
	doc, err := s.md.Parse("docs/evil.md", "docs/evil.html")
	if err != nil {
		t.Fatal(err)
	}
	html, err := doc.HTML()
	if err != nil {
		t.Fatal(err)
	}
	check(t, string(html))

	// And through a whole build.
	repo = fixtureRepo(t)
	writeFixture(t, repo, "docs/running.md", "# Running\n\n"+body)
	out := t.TempDir()
	if _, err := Build(Config{Repo: repo, Out: out}); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(out, "docs", "running.html"))
	if err != nil {
		t.Fatal(err)
	}
	check(t, string(page))
}
