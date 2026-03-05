// Generator for close-guarding wrapper methods.
//
// This program introspects the mobyClient.APIClient interface using reflection
// and generates wrapper methods for closeGuardingClient that acquire a read lock
// and check the closed flag before delegating to the underlying client.
//
// Usage: go run ./hack/cmd/gen-close-guard/
// (Run from pkg/docker/ directory, or via go generate)
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"

	mobyClient "github.com/moby/moby/client"
)

// knownAliases overrides the default import alias (last path component)
// for specific packages to match codebase conventions.
var knownAliases = map[string]string{
	"github.com/moby/moby/client": "mobyClient",
}

// importTracker tracks which packages are referenced by the generated code
// and assigns import aliases.
type importTracker struct {
	aliases map[string]string // pkgPath → alias
	used    map[string]string // pkgPath → alias (only packages actually referenced)
}

func newImportTracker() *importTracker {
	return &importTracker{
		aliases: make(map[string]string),
		used:    make(map[string]string),
	}
}

func (t *importTracker) resolve(pkgPath string) string {
	if alias, ok := t.used[pkgPath]; ok {
		return alias
	}

	alias, ok := t.aliases[pkgPath]
	if !ok {
		if known, ok := knownAliases[pkgPath]; ok {
			alias = known
		} else {
			alias = path.Base(pkgPath)
		}
		// Check for conflicts with existing aliases.
		for _, existing := range t.used {
			if existing == alias {
				parent := path.Base(path.Dir(pkgPath))
				alias = parent + capitalize(alias)
				break
			}
		}
		t.aliases[pkgPath] = alias
	}
	t.used[pkgPath] = alias
	return alias
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// typeStr converts a reflect.Type to its Go source representation,
// registering any package imports as needed.
func typeStr(t reflect.Type, tracker *importTracker) string {
	switch t.Kind() {
	case reflect.Pointer:
		return "*" + typeStr(t.Elem(), tracker)
	case reflect.Slice:
		return "[]" + typeStr(t.Elem(), tracker)
	case reflect.Array:
		return fmt.Sprintf("[%d]%s", t.Len(), typeStr(t.Elem(), tracker))
	case reflect.Map:
		return "map[" + typeStr(t.Key(), tracker) + "]" + typeStr(t.Elem(), tracker)
	case reflect.Chan:
		elem := typeStr(t.Elem(), tracker)
		switch t.ChanDir() {
		case reflect.RecvDir:
			return "<-chan " + elem
		case reflect.SendDir:
			return "chan<- " + elem
		default:
			return "chan " + elem
		}
	case reflect.Func:
		var ins, outs []string
		for i := 0; i < t.NumIn(); i++ {
			ins = append(ins, typeStr(t.In(i), tracker))
		}
		for i := 0; i < t.NumOut(); i++ {
			outs = append(outs, typeStr(t.Out(i), tracker))
		}
		s := "func(" + strings.Join(ins, ", ") + ")"
		switch len(outs) {
		case 0:
		case 1:
			s += " " + outs[0]
		default:
			s += " (" + strings.Join(outs, ", ") + ")"
		}
		return s
	case reflect.Interface:
		if t.Name() == "error" && t.PkgPath() == "" {
			return "error"
		}
		if t.PkgPath() != "" {
			return tracker.resolve(t.PkgPath()) + "." + t.Name()
		}
		return "any"
	default:
		if t.PkgPath() != "" {
			return tracker.resolve(t.PkgPath()) + "." + t.Name()
		}
		return t.Name()
	}
}

func main() {
	iface := reflect.TypeOf((*mobyClient.APIClient)(nil)).Elem()
	tracker := newImportTracker()

	type methodDef struct {
		name string
		code string
	}
	var methods []methodDef

	for i := 0; i < iface.NumMethod(); i++ {
		m := iface.Method(i)
		if m.Name == "Close" {
			continue
		}
		mt := m.Type

		// Build parameter list.
		var params []string
		var callArgs []string
		numIn := mt.NumIn()
		for j := 0; j < numIn; j++ {
			argName := fmt.Sprintf("arg%d", j)
			pt := mt.In(j)
			if mt.IsVariadic() && j == numIn-1 {
				// Variadic: reflect sees []T, we emit ...T.
				elemStr := typeStr(pt.Elem(), tracker)
				params = append(params, fmt.Sprintf("%s ...%s", argName, elemStr))
				callArgs = append(callArgs, argName+"...")
			} else {
				params = append(params, fmt.Sprintf("%s %s", argName, typeStr(pt, tracker)))
				callArgs = append(callArgs, argName)
			}
		}

		// Build return type list.
		var returns []string
		for j := 0; j < mt.NumOut(); j++ {
			returns = append(returns, typeStr(mt.Out(j), tracker))
		}

		returnStr := ""
		switch len(returns) {
		case 0:
		case 1:
			returnStr = " " + returns[0]
		default:
			returnStr = " (" + strings.Join(returns, ", ") + ")"
		}

		var buf bytes.Buffer
		fmt.Fprintf(&buf, "func (c *closeGuardingClient) %s(%s)%s {\n",
			m.Name, strings.Join(params, ", "), returnStr)
		fmt.Fprintf(&buf, "\tc.m.RLock()\n")
		fmt.Fprintf(&buf, "\tdefer c.m.RUnlock()\n")
		fmt.Fprintf(&buf, "\tif c.closed {\n")
		fmt.Fprintf(&buf, "\t\tpanic(\"use of closed client\")\n")
		fmt.Fprintf(&buf, "\t}\n")
		if len(returns) > 0 {
			fmt.Fprintf(&buf, "\treturn c.pimpl.%s(%s)\n", m.Name, strings.Join(callArgs, ", "))
		} else {
			fmt.Fprintf(&buf, "\tc.pimpl.%s(%s)\n", m.Name, strings.Join(callArgs, ", "))
		}
		fmt.Fprintf(&buf, "}\n")

		methods = append(methods, methodDef{m.Name, buf.String()})
	}

	// Sort by method name for deterministic output.
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].name < methods[j].name
	})

	// Build the output file.
	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by gen-close-guard; DO NOT EDIT.\n\n")
	fmt.Fprintf(&out, "package docker\n\n")

	// Write imports, separating stdlib from third-party.
	var stdlibLines, thirdPartyLines []string
	for pkgPath, alias := range tracker.used {
		baseName := path.Base(pkgPath)
		var line string
		if alias != baseName {
			line = fmt.Sprintf("\t%s %q", alias, pkgPath)
		} else {
			line = fmt.Sprintf("\t%q", pkgPath)
		}
		if !strings.Contains(pkgPath, ".") {
			stdlibLines = append(stdlibLines, line)
		} else {
			thirdPartyLines = append(thirdPartyLines, line)
		}
	}
	sort.Strings(stdlibLines)
	sort.Strings(thirdPartyLines)
	if len(stdlibLines) > 0 || len(thirdPartyLines) > 0 {
		fmt.Fprintf(&out, "import (\n")
		if len(stdlibLines) > 0 {
			fmt.Fprintf(&out, "%s\n", strings.Join(stdlibLines, "\n"))
		}
		if len(stdlibLines) > 0 && len(thirdPartyLines) > 0 {
			fmt.Fprintf(&out, "\n")
		}
		if len(thirdPartyLines) > 0 {
			fmt.Fprintf(&out, "%s\n", strings.Join(thirdPartyLines, "\n"))
		}
		fmt.Fprintf(&out, ")\n\n")
	}

	for _, m := range methods {
		fmt.Fprintf(&out, "%s\n", m.code)
	}

	// Format with gofmt.
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error formatting: %v\n", err)
		fmt.Fprintf(os.Stderr, "raw output:\n%s\n", out.String())
		os.Exit(1)
	}

	if err := os.WriteFile("zz_close_guarding_client_generated.go", formatted, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing file: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Generated %d methods\n", len(methods))
}
