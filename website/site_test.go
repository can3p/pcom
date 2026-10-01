package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
)

// The site is built once for the whole package, from the real repository: the
// coverage assertions below are made against what pcom ships, not against a
// fixture that can go stale.
var (
	buildOnce sync.Once
	builtSite *Site
	builtDir  string
	buildErr  error
)

func build(t *testing.T) (*Site, string) {
	t.Helper()
	buildOnce.Do(func() {
		builtDir, buildErr = os.MkdirTemp("", "pcom-site")
		if buildErr != nil {
			return
		}
		builtSite, buildErr = Build(Config{Repo: "..", Out: builtDir})
	})
	if buildErr != nil {
		t.Fatalf("build the site: %v", buildErr)
	}
	return builtSite, builtDir
}

func TestMain(m *testing.M) {
	code := m.Run()
	if builtDir != "" {
		_ = os.RemoveAll(builtDir)
	}
	os.Exit(code)
}

// fixtureRepo writes a minimal repository containing every file docPages
// names, so a test can break exactly one thing.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, d := range docPages {
		body := "# " + d.Nav + "\n\nFirst paragraph of " + d.Nav + ".\n"
		if d.Src == "README.md" {
			body = "# Fixture\n\nThe lede.\n\n## " + quickstartSection + "\n\nRun it.\n"
		}
		writeFixture(t, repo, d.Src, body)
	}
	return repo
}

func writeFixture(t *testing.T, repo, rel, body string) {
	t.Helper()
	full := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Every link the site emits must resolve: to a file it wrote, and to an
// anchor that exists on that page.
func TestEveryInternalLinkResolves(t *testing.T) {
	_, dir := build(t)
	problems, checked, err := checkLinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if checked < 20 {
		t.Fatalf("only %d links checked, the site cannot be complete", checked)
	}
	for _, p := range problems {
		t.Errorf("%s", p)
	}
}

// Repository files that are linked from the documentation but not published
// as pages become links to GitHub. The set is asserted so that a new one
// shows up here as a decision to make rather than as a 404.
func TestLinksToUnpublishedFilesAreKnown(t *testing.T) {
	site, _ := build(t)
	want := []string{}
	var got []string
	for repoPath := range site.Unpublished() {
		got = append(got, repoPath)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("links to unpublished files changed:\n got %v\nwant %v\n"+
			"either publish the file as a page or add it to this list deliberately", got, want)
	}
}

// Every docPages entry appears exactly once in the rendered sidebar, under
// its own group, guide first.
func TestSidebarListsEveryDocPageOnce(t *testing.T) {
	site, dir := build(t)
	if len(site.Nav) != 2 || site.Nav[0].Title != GroupGuide || site.Nav[1].Title != GroupDev {
		t.Fatalf("nav = %+v", site.Nav)
	}
	body, err := os.ReadFile(filepath.Join(dir, "readme.html"))
	if err != nil {
		t.Fatal(err)
	}
	sidebar := string(body)
	i := strings.Index(sidebar, `<details class="sidebar">`)
	j := strings.Index(sidebar, "</details>")
	if i < 0 || j < i {
		t.Fatal("no sidebar in readme.html")
	}
	sidebar = sidebar[i:j]
	for _, d := range docPages {
		want := fmt.Sprintf(`href="%s"`, relPath("readme.html", d.Out))
		if n := strings.Count(sidebar, want); n != 1 {
			t.Errorf("%s appears %d times in the sidebar, want 1", d.Out, n)
		}
	}
	if g, d := strings.Index(sidebar, "<h2>"+GroupGuide), strings.Index(sidebar, "<h2>"+GroupDev); g < 0 || d < g {
		t.Error("sidebar groups are not Guide then Developer docs")
	}
}

// Guide pages are discovered from disk: a file under docs/guide that docPages
// does not list fails here, as does a listed one that is gone.
func TestGuidePagesOnDiskMatchDocPages(t *testing.T) {
	site, dir := build(t)
	files, err := filepath.Glob(filepath.Join("..", "docs", "guide", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk, listed []string
	for _, f := range files {
		onDisk = append(onDisk, "docs/guide/"+filepath.Base(f))
	}
	for _, d := range docPages {
		if d.Group == GroupGuide {
			listed = append(listed, d.Src)
		}
	}
	sort.Strings(onDisk)
	sort.Strings(listed)
	if strings.Join(onDisk, ",") != strings.Join(listed, ",") {
		t.Errorf("guide files on disk and in docPages differ:\n disk   %v\n listed %v", onDisk, listed)
	}

	body, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	var hrefs []string
	for _, m := range regexp.MustCompile(`(?s)<div class="card">.*?<h3><a href="([^"]+)"`).FindAllStringSubmatch(string(body), -1) {
		hrefs = append(hrefs, m[1])
	}
	var want []string
	for _, p := range site.guidePages() {
		want = append(want, p.Path)
	}
	for i, f := range onDisk {
		onDisk[i] = strings.TrimSuffix(f, ".md") + ".html"
	}
	sort.Strings(hrefs)
	sort.Strings(want)
	if strings.Join(hrefs, ",") != strings.Join(onDisk, ",") || strings.Join(want, ",") != strings.Join(onDisk, ",") {
		t.Errorf("landing cards %v, guide pages %v, files on disk %v", hrefs, want, onDisk)
	}
}

// The landing page has one card per guide page, built from the page itself.
func TestLandingPageHasOneCardPerGuidePage(t *testing.T) {
	site, dir := build(t)
	body, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	guide := site.guidePages()
	if len(guide) == 0 {
		t.Fatal("no guide pages")
	}
	if n := strings.Count(string(body), `<div class="card">`); n != len(guide) {
		t.Errorf("%d cards, want %d", n, len(guide))
	}
	for _, p := range guide {
		if !strings.Contains(string(body), `href="`+p.Path+`"`) {
			t.Errorf("the landing page has no card linking %s", p.Path)
		}
	}
}

func TestLandingCardsWithAndWithoutImage(t *testing.T) {
	repo := fixtureRepo(t)
	writeFixture(t, repo, "docs/guide/pic.md", "# Pic\n\nHas a picture.\n\n![shot](screenshots/a.png)\n")
	writeFixture(t, repo, "docs/guide/screenshots/a.png", "png")
	writeFixture(t, repo, "docs/guide/bare.md", "# Bare\n\nNo picture.\n")
	s := newSite(repo)
	for _, n := range []string{"pic", "bare"} {
		s.Pages = append(s.Pages, &Page{
			Path: "docs/guide/" + n + ".html", Group: GroupGuide, Kind: KindMarkdown, Src: "docs/guide/" + n + ".md",
		})
	}
	v, err := s.landing()
	if err != nil {
		t.Fatal(err)
	}
	if want := len(s.guidePages()); len(v.Cards) != want {
		t.Fatalf("cards = %+v, want %d", v.Cards, want)
	}
	byTitle := map[string]GuideCard{}
	for _, c := range v.Cards {
		byTitle[c.Title] = c
	}
	if c := byTitle["Pic"]; c.Image != "docs/guide/screenshots/a.png" || c.Alt != "shot" ||
		!strings.Contains(string(c.Intro), "Has a picture.") {
		t.Errorf("Pic card = %+v", c)
	}
	if c := byTitle["Bare"]; c.Image != "" || !strings.Contains(string(c.Intro), "No picture.") {
		t.Errorf("Bare card = %+v", c)
	}
}

func TestBuildFailsForAMissingPage(t *testing.T) {
	repo := fixtureRepo(t)
	if err := os.Remove(filepath.Join(repo, "docs", "lessons.md")); err != nil {
		t.Fatal(err)
	}
	_, err := Build(Config{Repo: repo, Out: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "lessons.md") {
		t.Fatalf("want an error naming lessons.md, got %v", err)
	}
}

func TestBuildFailsForAMissingImage(t *testing.T) {
	repo := fixtureRepo(t)
	writeFixture(t, repo, "docs/running.md", "# Running\n\n![gone](nope/gone.png)\n")
	_, err := Build(Config{Repo: repo, Out: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "gone.png") {
		t.Fatalf("want an error naming gone.png, got %v", err)
	}
}

func TestBuildCopiesImages(t *testing.T) {
	repo := fixtureRepo(t)
	writeFixture(t, repo, "docs/guide/overview.md", "# Overview\n\nIntro.\n\n![a](screenshots/a.png)\n")
	writeFixture(t, repo, "docs/guide/screenshots/a.png", "png")
	out := t.TempDir()
	if _, err := Build(Config{Repo: repo, Out: out}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"docs/guide/screenshots/a.png", "style.css", ".nojekyll", "index.html"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(f))); err != nil {
			t.Errorf("output lacks %s: %v", f, err)
		}
	}
	idx, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil || !strings.Contains(string(idx), `src="docs/guide/screenshots/a.png"`) {
		t.Errorf("landing card image link is wrong: %v", err)
	}
	if problems, _, err := checkLinks(out); err != nil || len(problems) != 0 {
		t.Errorf("links do not resolve: %v %v", problems, err)
	}
	body, err := os.ReadFile(filepath.Join(out, "docs", "guide", "overview.html"))
	if err != nil || !strings.Contains(string(body), `src="screenshots/a.png"`) {
		t.Errorf("image link not relative to the page: %v", err)
	}
}

// --- the link checker, used by the test above and tested itself -----------

var linkRE = regexp.MustCompile(`(?:href|src)="([^"]*)"`)

// checkLinks walks the generated site and reports every internal link that
// does not resolve to a file, or to an anchor on the page it names.
func checkLinks(dir string) (problems []string, checked int, err error) {
	pages := map[string]string{}
	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".html") {
			return err
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		pages[filepath.ToSlash(rel)] = string(body)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	ids := map[string]map[string]bool{}
	idRE := regexp.MustCompile(`id="([^"]+)"`)
	for name, body := range pages {
		set := map[string]bool{}
		for _, m := range idRE.FindAllStringSubmatch(body, -1) {
			set[m[1]] = true
		}
		ids[name] = set
	}
	names := make([]string, 0, len(pages))
	for name := range pages {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, m := range linkRE.FindAllStringSubmatch(pages[name], -1) {
			href := m[1]
			if href == "" || strings.HasPrefix(href, "//") {
				continue
			}
			if i := strings.Index(href, ":"); i > 0 && !strings.ContainsAny(href[:i], "/.") {
				continue // external scheme
			}
			checked++
			target, frag := href, ""
			if before, after, ok := strings.Cut(href, "#"); ok {
				target, frag = before, after
			}
			page := name
			if target != "" {
				page = path.Clean(path.Join(path.Dir(name), target))
				fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(page)))
				if err != nil {
					problems = append(problems, fmt.Sprintf("%s links to %s, which the site does not contain", name, href))
					continue
				}
				if fi.IsDir() {
					problems = append(problems, fmt.Sprintf("%s links to %s, which is a directory, not a file", name, href))
					continue
				}
			}
			if frag != "" && !ids[page][frag] {
				problems = append(problems, fmt.Sprintf("%s links to %s, but %s has no such anchor", name, href, page))
			}
		}
	}
	return problems, checked, nil
}

func TestCheckLinksCatchesABrokenLink(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.html", `<a href="b.html">ok</a><a href="b.html#top">ok</a>`+
		`<a href="gone.html">bad</a><a href="b.html#nope">bad</a>`+
		`<a href="https://example.com/x.html">external</a><a href="sub">dir</a>`)
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	write("b.html", `<h1 id="top">b</h1>`)

	problems, checked, err := checkLinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 5 {
		t.Errorf("checked %d internal links, want 5", checked)
	}
	if len(problems) != 3 {
		t.Fatalf("want 3 problems, got %d: %v", len(problems), problems)
	}
	if !strings.Contains(problems[0], "gone.html") || !strings.Contains(problems[1], "no such anchor") ||
		!strings.Contains(problems[2], "directory") {
		t.Errorf("unexpected problems: %v", problems)
	}
}

func TestBuildFailsForBrokenLandingSources(t *testing.T) {
	cases := []struct {
		name, file, body, want string
	}{
		{"guide page without an H1", "docs/guide/overview.md", "Just a paragraph.\n", "H1"},
		{"guide page without a paragraph", "docs/guide/overview.md", "# Overview\n", "paragraph"},
		{"README without the quick start", "README.md", "# Fixture\n\nThe lede.\n", quickstartSection},
		{"a link above the repository root", "docs/running.md", "# R\n\n[x](../../outside.md)\n", "climbs"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := fixtureRepo(t)
			writeFixture(t, repo, c.file, c.body)
			_, err := Build(Config{Repo: repo, Out: t.TempDir()})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want an error containing %q, got %v", c.want, err)
			}
		})
	}
}
