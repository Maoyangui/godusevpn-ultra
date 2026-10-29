package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// Optional controls must never enter the tunnel lifecycle, including indirectly
// by reusing an IPC connect/disconnect command after a policy error.
func TestPrivacyPlaneHasNoTunnelMutationOrHTTPDependency(t *testing.T) {
	files, err := filepath.Glob("privacy*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			for _, bad := range []string{"/internal/daemon", "/internal/core", "/internal/builder", "/internal/state", "\"net/http\""} {
				if strings.Contains(imp.Path.Value, bad) {
					t.Errorf("%s imported %s", name, imp.Path.Value)
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "Connect", "Disconnect", "SetMode", "SaveSettings", "ApplyGuard", "ClearGuard":
				// The location portal has its own explicit local HTTPS session;
				// it must never be confused with the tunnel lifecycle checked here.
				if name == "privacy_location_portal.go" && sel.Sel.Name == "Connect" {
					return true
				}
				t.Errorf("%s calls tunnel mutator %s", name, sel.Sel.Name)
			}
			return true
		})
	}
}
