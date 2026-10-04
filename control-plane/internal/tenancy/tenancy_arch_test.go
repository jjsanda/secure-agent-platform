package tenancy_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoRawPoolOutsideTenancy enforces the architecture invariant this package
// documents: only the db package (which constructs the pool) and the tenancy
// package (the WithTenant choke point) may import pgxpool directly. Every other
// internal package must go through tenancy.Store, so row-level security's
// per-request tenant context can never be forgotten in a hand-written query.
//
// The build genuinely fails if someone reaches for the raw pool elsewhere — the
// claim in README/ADR-0002/CONTRIBUTING is backed by this test.
func TestNoRawPoolOutsideTenancy(t *testing.T) {
	const banned = "github.com/jackc/pgx/v5/pgxpool"

	internalRoot, err := filepath.Abs("..") // this test runs in internal/tenancy
	if err != nil {
		t.Fatal(err)
	}

	allowed := func(path string) bool {
		return strings.Contains(path, string(filepath.Separator)+"db"+string(filepath.Separator)) ||
			strings.Contains(path, string(filepath.Separator)+"tenancy"+string(filepath.Separator))
	}

	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(internalRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if allowed(path) {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, `"`) == banned {
				rel, _ := filepath.Rel(internalRoot, path)
				t.Errorf("internal/%s imports %s directly; use tenancy.Store (WithTenant) so RLS context is always set", rel, banned)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
}
