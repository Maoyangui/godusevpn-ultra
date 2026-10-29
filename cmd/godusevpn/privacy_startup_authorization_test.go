package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

// This is a source-shape regression test: starting the ordinary GUI must not
// request UAC or create the privileged privacy helper. The user must explicitly
// invoke PrivacyAuthorize from the privacy/browser action. No process, token,
// pipe, UAC dialog, network or system setting is touched.
func TestStartupDoesNotAuthorizePrivacyHelper(t *testing.T) {
	source, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "app.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	var startup *ast.FuncDecl
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "startup" {
			startup = function
			break
		}
	}
	if startup == nil || startup.Body == nil {
		t.Fatal("startup function missing")
	}
	foundAutomaticAuthorization := false
	ast.Inspect(startup.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "startPrivacyHost" {
			foundAutomaticAuthorization = true
		}
		return true
	})
	if foundAutomaticAuthorization {
		t.Fatal("startup must not request the privileged privacy helper")
	}

	privacySource, err := os.ReadFile("privacy.go")
	if err != nil {
		t.Fatal(err)
	}
	privacyFile, err := parser.ParseFile(token.NewFileSet(), "privacy.go", privacySource, 0)
	if err != nil {
		t.Fatal(err)
	}
	foundExplicitEntry := false
	for _, declaration := range privacyFile.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "PrivacyAuthorize" {
			foundExplicitEntry = true
			break
		}
	}
	if !foundExplicitEntry {
		t.Fatal("explicit PrivacyAuthorize entry is missing")
	}
}
