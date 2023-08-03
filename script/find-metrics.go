// Usage: go run script/find-metrics.go > metrics.md
//
// This script finds all of the metrics in the Syncthing codebase and prints
// them in Markdown format. It's used to generate the metrics documentation
// for the Syncthing docs.
package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"strconv"
	"strings"

	"golang.org/x/exp/slices"
	"golang.org/x/tools/go/packages"
)

type metric struct {
	subsystem string
	name      string
	help      string
	kind      string
}

func main() {
	opts := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
	}

	pkgs, err := packages.Load(opts, "github.com/syncthing/syncthing/...")
	if err != nil {
		log.Fatalln(err)
	}

	var coll metricCollector
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, coll.Visit)
		}
	}
	coll.print()
}

type metricCollector struct {
	metrics []metric
}

func (c *metricCollector) Visit(n ast.Node) bool {
	if gen, ok := n.(*ast.GenDecl); ok {
		// We're only interested in var declarations (var metricWhatever =
		// promauto.NewCounter(...) etc).
		if gen.Tok != token.VAR {
			return false
		}

		for _, spec := range gen.Specs {
			// We want to look at the value given to a var (the NewCounter()
			// etc call).
			if vsp, ok := spec.(*ast.ValueSpec); ok {
				// There should be only one value.
				if len(vsp.Values) != 1 {
					continue
				}

				// The value should be a function call.
				call, ok := vsp.Values[0].(*ast.CallExpr)
				if !ok {
					continue
				}

				// The call should be a selector expression
				// (package.Identifer).
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					continue
				}

				// The package selector should be `promauto`.
				selID, ok := sel.X.(*ast.Ident)
				if !ok || selID.Name != "promauto" {
					continue
				}

				// The function should be one of the New* functions.
				var kind string
				switch sel.Sel.Name {
				case "NewCounter":
					kind = "counter"
				case "NewGauge":
					kind = "gauge"
				case "NewCounterVec":
					kind = "counter vector"
				case "NewGaugeVec":
					kind = "gauge vector"
				default:
					continue
				}

				// The arguments to the function should be a single
				// composite (struct literal). Grab all of the fields in the
				// declaration into a map so we can easily access them.
				args := make(map[string]string)
				for _, el := range call.Args[0].(*ast.CompositeLit).Elts {
					kv := el.(*ast.KeyValueExpr)
					key := kv.Key.(*ast.Ident).Name       // e.g., "Name"
					val := kv.Value.(*ast.BasicLit).Value // e.g., `"foo"`
					args[key], _ = strconv.Unquote(val)
				}

				// Build the full name of the metric from the namespace +
				// subsystem + name, like Prometheus does.
				var parts []string
				if v := args["Namespace"]; v != "" {
					parts = append(parts, v)
				}
				if v := args["Subsystem"]; v != "" {
					parts = append(parts, v)
				}
				if v := args["Name"]; v != "" {
					parts = append(parts, v)
				}
				fullName := strings.Join(parts, "_")

				// Add the metric to the list.
				c.metrics = append(c.metrics, metric{
					subsystem: args["Subsystem"],
					name:      fullName,
					help:      args["Help"],
					kind:      kind,
				})
			}
		}
	}
	return true
}

func (c *metricCollector) print() {
	slices.SortFunc(c.metrics, func(a, b metric) bool {
		if a.subsystem != b.subsystem {
			return a.subsystem < b.subsystem
		}
		return a.name < b.name
	})

	var prevSubsystem string
	for _, m := range c.metrics {
		if m.subsystem != prevSubsystem {
			fmt.Printf("## Package `%s`\n\n", m.subsystem)
			prevSubsystem = m.subsystem
		}
		fmt.Printf("### `%v` (%s)\n\n%s\n\n", m.name, m.kind, wordwrap(sentenceize(m.help), 72))
	}
}

func sentenceize(s string) string {
	if s == "" {
		return ""
	}
	if !strings.HasSuffix(s, ".") {
		return s + "."
	}
	return s
}

func wordwrap(s string, width int) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		for len(line) > width {
			i := strings.LastIndex(line[:width], " ")
			if i == -1 {
				i = width
			}
			lines = append(lines, line[:i])
			line = line[i+1:]
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
