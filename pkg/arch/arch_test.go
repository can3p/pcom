// Package arch enforces the layering in docs/architecture.md: queries live in
// pkg/repo, services don't speak HTTP, and transports don't see repositories.
package arch_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

const module = "github.com/can3p/pcom"

// exempt packages may use the database directly: the repositories, the
// generated models, test fixtures and harnesses, the session store and the
// mail queue (infrastructure that owns its table), the seed command and the
// composition root.
var exempt = []string{
	"pkg/model/core",
	"pkg/repo",
	"pkg/testutil/",
	"pkg/feedops/testutil",
	"pkg/pgsession",
	"pkg/mail/sender/dbsender",
	"pkg/service/registry",
	"cmd/web/cmd_seed.go",
	"cmd/web/serve.go",
	"cmd/web/db.go",
	"pkg/web/app/deps.go",
	"e2e",
}

// ormPackages are the database libraries. Importing one, calling into one, or
// calling anything whose signature carries one of their types is database work.
var ormPackages = []string{
	"github.com/volatiletech/sqlboiler/v4/",
	"github.com/jmoiron/sqlx",
	"database/sql",
}

// coreQuery matches the generated query types (core.postQuery, ...).
var coreQuery = regexp.MustCompile(regexp.QuoteMeta(module+"/pkg/model/core.") + `[a-z]\w*Query\b`)

// callTargets may be handed a database handle from anywhere: they are the
// composition root's building blocks.
var callTargets = []string{
	module + "/pkg/pgsession",
	module + "/pkg/service/registry",
}

// allowlist is every file that breaks the rules today, with the RS task that
// fixes it. A task deletes its entries; RS is done when this is empty. The
// test fails on an entry that no longer breaks a rule, so the list only shrinks.
var allowlist = map[string]string{}

func TestLayering(t *testing.T) {
	t.Parallel()

	got := violations(t)

	var files []string
	for f := range got {
		files = append(files, f)
	}
	sort.Strings(files)

	for _, f := range files {
		if _, ok := allowlist[f]; ok {
			continue
		}

		t.Errorf("%s breaks the layering (docs/architecture.md):\n\t%s", f, strings.Join(first(got[f], 5), "\n\t"))
	}

	for f, task := range allowlist {
		if _, ok := got[f]; !ok {
			t.Errorf("%s follows the layering now: delete its allowlist entry (%s)", f, task)
		}
	}
}

// violations maps a repo-relative file to what is wrong with it.
func violations(t *testing.T) map[string][]string {
	t.Helper()

	pkgs := goList(t)

	exports := map[string]string{}
	for _, p := range pkgs {
		exports[p.ImportPath] = p.Export
	}

	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		if exports[path] == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(exports[path])
	})

	out := map[string][]string{}
	add := func(pos token.Pos, format string, args ...any) {
		p := fset.Position(pos)
		rel := strings.TrimPrefix(p.Filename, repoRoot(t)+"/")
		if isExempt(rel) {
			return // type-checked with its package, never reported
		}
		out[rel] = append(out[rel], fmt.Sprintf("line %d: ", p.Line)+fmt.Sprintf(format, args...))
	}

	for _, p := range pkgs {
		rel, ok := strings.CutPrefix(p.ImportPath, module+"/")
		if !ok || p.ImportPath == module || isExempt(rel) || len(p.GoFiles) == 0 {
			continue
		}

		var files []*ast.File
		for _, name := range p.GoFiles {
			f, err := parser.ParseFile(fset, filepath.Join(p.Dir, name), nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, f)
		}

		info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
		conf := types.Config{Importer: imp, Error: func(error) {}}
		if _, err := conf.Check(p.ImportPath, fset, files, info); err != nil {
			t.Fatalf("type-checking %s: %v", p.ImportPath, err)
		}

		for _, f := range files {
			for _, spec := range f.Imports {
				path := strings.Trim(spec.Path.Value, `"`)
				if why := forbiddenImport(rel, path); why != "" {
					add(spec.Pos(), "imports %s: %s", path, why)
				}
			}
		}

		for id, obj := range info.Uses {
			fn, ok := obj.(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() == p.ImportPath || slices.Contains(callTargets, fn.Pkg().Path()) {
				continue
			}

			if isORM(fn.Pkg().Path()) || mentionsORM(fn.Type().(*types.Signature)) {
				add(id.Pos(), "calls %s.%s, which does database work", fn.Pkg().Name(), fn.Name())
			}
		}
	}

	for f := range out {
		sort.Strings(out[f])
	}

	return out
}

func forbiddenImport(rel, path string) string {
	switch {
	case isORM(path):
		return "only repositories query the database"
	case path == module+"/pkg/repo":
		if !strings.HasPrefix(rel, "pkg/service/") && !strings.HasPrefix(rel, "cmd/") && rel != "pkg/mail/sender/dbsender" {
			return "only services use repositories"
		}
	case strings.HasPrefix(rel, "pkg/service/") && (path == "github.com/gin-gonic/gin" || path == "net/http"):
		return "services don't speak HTTP"
	}

	return ""
}

func isORM(path string) bool {
	for _, p := range ormPackages {
		if path == strings.TrimSuffix(p, "/") || strings.HasPrefix(path, p) {
			return true
		}
	}

	return false
}

func mentionsORM(sig *types.Signature) bool {
	var s []string
	for _, tuple := range []*types.Tuple{sig.Params(), sig.Results()} {
		for v := range tuple.Variables() {
			s = append(s, types.TypeString(v.Type(), nil))
		}
	}

	all := strings.Join(s, " ")
	for _, p := range ormPackages {
		if strings.Contains(all, strings.TrimSuffix(p, "/")) {
			return true
		}
	}

	return coreQuery.MatchString(all)
}

func isExempt(rel string) bool {
	for _, e := range exempt {
		if rel == e || strings.HasPrefix(rel, strings.TrimSuffix(e, "/")+"/") {
			return true
		}
	}

	return false
}

type listedPackage struct {
	ImportPath string
	Dir        string
	Export     string
	GoFiles    []string
}

func goList(t *testing.T) []listedPackage {
	t.Helper()

	cmd := exec.Command("go", "list", "-export", "-deps", "-json=ImportPath,Dir,Export,GoFiles", "./...")
	cmd.Dir = repoRoot(t)
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}

	var pkgs []listedPackage
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for {
		var p listedPackage
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		pkgs = append(pkgs, p)
	}

	return pkgs
}

func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	return root
}

func first(s []string, n int) []string {
	if len(s) > n {
		return append(s[:n:n], fmt.Sprintf("... and %d more", len(s)-n))
	}

	return s
}

const awsConfigPkg = "github.com/aws/aws-sdk-go-v2/config"

// envExempt may read the environment: the composition root that parses the
// settings, and test harnesses and fixtures.
var envExempt = []string{"cmd/web", "e2e", "pkg/testutil"}

// envAllowlist is every file that still reads the environment itself, with
// the R2 task that moves it onto pkg/config. R2 is done when this is empty.
var envAllowlist = map[string]string{}

// TestSettingsComeFromConfig keeps every setting in pkg/config, parsed by
// go-flags: no package reads the environment on its own.
func TestSettingsComeFromConfig(t *testing.T) {
	t.Parallel()

	got := map[string][]string{}
	fset := token.NewFileSet()

	for _, p := range goList(t) {
		rel, ok := strings.CutPrefix(p.ImportPath, module+"/")
		if !ok {
			continue
		}

		exempt := slices.ContainsFunc(envExempt, func(e string) bool { return rel == e || strings.HasPrefix(rel, e+"/") })

		for _, name := range p.GoFiles {
			path := filepath.Join(p.Dir, name)

			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}

			// the AWS config loader reads AWS_* variables and ~/.aws
			if rel != "cmd/web" && !strings.HasPrefix(rel, "cmd/web/") && importName(f, awsConfigPkg) != "" {
				t.Errorf("%s imports %s, which reads the environment; build the client from explicit settings", strings.TrimPrefix(path, repoRoot(t)+"/"), awsConfigPkg)
			}

			osName := importName(f, "os")
			if osName == "" || exempt {
				continue
			}

			ast.Inspect(f, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				if x, ok := sel.X.(*ast.Ident); ok && x.Name == osName && slices.Contains([]string{"Getenv", "LookupEnv", "Environ"}, sel.Sel.Name) {
					file := strings.TrimPrefix(path, repoRoot(t)+"/")
					got[file] = append(got[file], fmt.Sprintf("line %d: os.%s", fset.Position(sel.Pos()).Line, sel.Sel.Name))
				}

				return true
			})
		}
	}

	for f, where := range got {
		if _, ok := envAllowlist[f]; !ok {
			t.Errorf("%s reads the environment; declare the setting in pkg/config instead:\n\t%s", f, strings.Join(first(where, 5), "\n\t"))
		}
	}

	for f, task := range envAllowlist {
		if _, ok := got[f]; !ok {
			t.Errorf("%s no longer reads the environment: delete its envAllowlist entry (%s)", f, task)
		}
	}
}

// importName is the name f refers to the package path by, or "" when f
// doesn't import it.
func importName(f *ast.File, path string) string {
	for _, spec := range f.Imports {
		if strings.Trim(spec.Path.Value, `"`) != path {
			continue
		}

		if spec.Name != nil {
			return spec.Name.Name
		}

		return filepath.Base(path)
	}

	return ""
}
