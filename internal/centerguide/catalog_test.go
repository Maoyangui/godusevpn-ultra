package centerguide

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Maoyangui/godusevpn/internal/metadatatool"
)

func TestGuidesOnlyDeclareBoundedHelpAndFixedExistingNavigation(t *testing.T) {
	catalog := Guides()
	if catalog.SchemaVersion != 1 || catalog.Kind != "help_only_not_live_evidence" || len(catalog.Modules) != 14 {
		t.Fatal("wrong help contract")
	}
	allowed := map[string]map[string]bool{
		"settings":    {"dateandtime": true, "language": true, "privacy-location": true, "camera": true, "microphone": true, "privacy": true, "windowsdefender": true, "network-wifi": true},
		"privacy-tab": {"system": true, "network": true, "data": true},
		"app-page":    {"settings": true, "profiles": true, "logs": true, "detection": true},
	}
	seen := map[string]bool{}
	for _, module := range catalog.Modules {
		if seen[module.ID] || module.ID == "" || module.Title == "" || module.Scope == "" || module.Verification == "" || module.Recovery == "" || len(module.Limitations) == 0 || len(module.Pending) == 0 || utf8.RuneCountInString(module.Summary) > 60 {
			t.Fatal("missing, duplicate or unbounded guide", module.ID)
		}
		seen[module.ID] = true
		for _, method := range append(module.Backend.Read, module.Backend.Write...) {
			if strings.Contains(module.Verification+module.Recovery+module.Summary+strings.Join(module.Limitations, " "), method) {
				t.Fatal("user help exposes internal method name", module.ID, method)
			}
		}
		switch module.Capability {
		case "limited", "backend":
			if len(module.Backend.Read) == 0 || len(module.Backend.Write) == 0 {
				t.Fatal("backend capability lacks real operations", module.ID)
			}
		case "settings", "pending":
			if len(module.Backend.Write) != 0 {
				t.Fatal("unimplemented module advertised a write", module.ID)
			}
		default:
			t.Fatal("unknown capability", module.Capability)
		}
		for _, action := range module.Actions {
			if !allowed[action.Kind][action.Target] || action.Label == "" || strings.ContainsAny(action.Target, "/\\:\r\n") {
				t.Fatal("uncontrolled action", action)
			}
		}
	}
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"canApply":`, `"protected":`, `"generatedAt":`, `"detectedAt":`, `"admin":`, `"available":`} {
		if strings.Contains(string(encoded), key) {
			t.Fatal("static help pretends to provide live state", key)
		}
	}
}

func TestGuideMethodsAndSettingsAreRealProductEntrypoints(t *testing.T) {
	// Parse source only. Never instantiate App/managers or call a native API.
	root := filepath.Join("..", "..")
	files, err := filepath.Glob(filepath.Join(root, "cmd", "godusevpn", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	methods := map[string]bool{}
	settings := map[string]bool{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			method, ok := declaration.(*ast.FuncDecl)
			if !ok || method.Recv == nil || len(method.Recv.List) != 1 {
				continue
			}
			pointer, ok := method.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			if typ, ok := pointer.X.(*ast.Ident); ok && typ.Name == "App" {
				methods[method.Name.Name] = true
			}
		}
		if filepath.Base(name) != "privacy_settings_windows.go" {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.ValueSpec)
			if !ok || len(spec.Names) != 1 || spec.Names[0].Name != "privacySettingPages" || len(spec.Values) != 1 {
				return true
			}
			literal, ok := spec.Values[0].(*ast.CompositeLit)
			if !ok {
				return false
			}
			for _, entry := range literal.Elts {
				if pair, ok := entry.(*ast.KeyValueExpr); ok {
					if key, ok := pair.Key.(*ast.BasicLit); ok {
						value, err := strconv.Unquote(key.Value)
						if err == nil {
							settings[value] = true
						}
					}
				}
			}
			return false
		})
	}
	for _, module := range Guides().Modules {
		for _, method := range append(module.Backend.Read, module.Backend.Write...) {
			if !methods[method] {
				t.Fatal("guide points to missing backend", module.ID, method)
			}
		}
		for _, action := range module.Actions {
			if action.Kind == "settings" && !settings[action.Target] {
				t.Fatal("guide points to unknown settings page", action.Target)
			}
		}
	}
	// Actual navigation keys are asserted from the shipped frontend source.
	app, err := os.ReadFile(filepath.Join(root, "web", "dist", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{"settings", "profiles", "logs", "detection"} {
		if !strings.Contains(string(app), page+": render") {
			t.Fatal("missing page", page)
		}
	}
}

func TestGuidesMatchActualMetadataAndSystemPolicySelection(t *testing.T) {
	var metadata, policies Module
	for _, module := range Guides().Modules {
		switch module.ID {
		case "image-metadata":
			metadata = module
		case "system-policies":
			policies = module
		}
	}
	var fields []string
	for _, field := range metadatatool.Fields() {
		fields = append(fields, field.ID)
	}
	if !reflect.DeepEqual(metadata.SelectionIDs, fields) {
		t.Fatal("help exceeds actual EXIF selection")
	}
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "privacy", "catalog.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "addSystem" {
			if arg, ok := call.Args[0].(*ast.BasicLit); ok {
				id, err := strconv.Unquote(arg.Value)
				if err == nil {
					ids[id] = true
				}
			}
		}
		return true
	})
	if len(ids) != len(policies.ControlIDs) {
		t.Fatal("system policy count differs")
	}
	for _, id := range policies.ControlIDs {
		if !ids[id] {
			t.Fatal("unknown policy", id)
		}
	}
}

func TestGuidesReturnFreshNestedValues(t *testing.T) {
	first := Guides()
	first.Modules[0].Title = "changed"
	first.Modules[0].ControlIDs[0] = "changed"
	first.Modules[0].Backend.Write[0] = "changed"
	first.Modules[0].Actions[0].Target = "changed"
	first.Modules[0].Limitations[0] = "changed"
	second, _ := json.Marshal(Guides())
	if strings.Contains(string(second), "changed") {
		t.Fatal("caller modified shared catalog state")
	}
}
