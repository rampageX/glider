package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Every public main/subscribe config option must be listed in the readable
// reference file, so future config changes cannot silently omit documentation.
func TestInstanceConfigReferenceCoversRegisteredOptions(t *testing.T) {
	raw, err := os.ReadFile("config/subscription/instance.reference.conf.example")
	if err != nil {
		t.Fatal(err)
	}
	doc := "\n" + string(raw)
	for _, source := range []string{"config.go", "subscription/config.go"} {
		node, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			recv, ok := fn.X.(*ast.Ident)
			if !ok {
				return true
			}
			index := -1
			switch recv.Name {
			case "flag", "f":
				if strings.HasSuffix(fn.Sel.Name, "Var") {
					index = 1
				} else if fn.Sel.Name == "Bool" || fn.Sel.Name == "String" {
					index = 0
				}
			case "timevalue":
				if strings.HasSuffix(fn.Sel.Name, "Var") {
					index = 2
				}
			}
			if index < 0 || len(call.Args) <= index {
				return true
			}
			lit, ok := call.Args[index].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			name, err := strconv.Unquote(lit.Value)
			if err != nil || name == "scheme" || name == "example" {
				return true
			}
			if !strings.Contains(doc, "\n# "+name+"=") {
				t.Errorf("%s: config option %q missing from instance reference template", source, name)
			}
			return true
		})
	}
}
