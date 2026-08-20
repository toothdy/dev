package codegen

import (
	"go/types"
	"sort"
	"strconv"
)

type importManager struct {
	aliases map[string]string
	paths   map[string]string
}

func newImportManager() *importManager {
	return &importManager{aliases: make(map[string]string), paths: make(map[string]string)}
}

func (m *importManager) add(path, preferred string) {
	if path == "" || path == "builtin" {
		return
	}
	if _, exists := m.paths[path]; !exists {
		m.paths[path] = preferred
	}
}

func (m *importManager) finalize() {
	if len(m.aliases) > 0 {
		return
	}
	fixed := []struct {
		alias string
		path  string
	}{
		{alias: "module", path: modulePackagePath},
		{alias: "corecontroller", path: controllerPackagePath},
		{alias: "coreroute", path: routePackagePath},
		{alias: "coreentity", path: entityPackagePath},
		{alias: "coreservice", path: servicePackagePath},
		{alias: "coredb", path: databasePackagePath},
		{alias: "corerecycle", path: recyclePackagePath},
		{alias: "gdb", path: gdbPackagePath},
		{alias: "g", path: gPackagePath},
	}
	used := make(map[string]bool, len(m.paths))
	for _, item := range fixed {
		if _, exists := m.paths[item.path]; !exists {
			continue
		}
		m.aliases[item.path] = item.alias
		used[item.alias] = true
	}
	paths := make([]string, 0, len(m.paths))
	for path := range m.paths {
		if _, fixed := m.aliases[path]; !fixed {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		alias := m.paths[path]
		if alias == "" {
			alias = "pkg"
		}
		base := alias
		for suffix := 2; used[alias]; suffix++ {
			alias = base + strconv.Itoa(suffix)
		}
		m.aliases[path] = alias
		used[alias] = true
	}
}

func (m *importManager) alias(path string) string { return m.aliases[path] }

func (m *importManager) pathsInOrder() []string {
	paths := make([]string, 0, len(m.paths))
	for path := range m.paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func collectTypeImports(manager *importManager, value types.Type) {
	seen := make(map[types.Type]bool)
	collectTypeImportsInto(manager, value, seen)
}

func collectTypeImportsInto(manager *importManager, value types.Type, seen map[types.Type]bool) {
	if value == nil || seen[value] {
		return
	}
	seen[value] = true
	switch current := value.(type) {
	case *types.Alias:
		if object := current.Obj(); object != nil && object.Pkg() != nil {
			manager.add(object.Pkg().Path(), object.Pkg().Name())
		}
		collectTypeImportsInto(manager, types.Unalias(current), seen)
	case *types.Array:
		collectTypeImportsInto(manager, current.Elem(), seen)
	case *types.Chan:
		collectTypeImportsInto(manager, current.Elem(), seen)
	case *types.Interface:
		for index := range current.NumMethods() {
			collectTypeImportsInto(manager, current.Method(index).Type(), seen)
		}
	case *types.Map:
		collectTypeImportsInto(manager, current.Key(), seen)
		collectTypeImportsInto(manager, current.Elem(), seen)
	case *types.Named:
		if object := current.Obj(); object != nil && object.Pkg() != nil {
			manager.add(object.Pkg().Path(), object.Pkg().Name())
		}
		for index := range current.TypeArgs().Len() {
			collectTypeImportsInto(manager, current.TypeArgs().At(index), seen)
		}
	case *types.Pointer:
		collectTypeImportsInto(manager, current.Elem(), seen)
	case *types.Signature:
		collectTupleImports(manager, current.Params(), seen)
		collectTupleImports(manager, current.Results(), seen)
	case *types.Slice:
		collectTypeImportsInto(manager, current.Elem(), seen)
	case *types.Struct:
		for index := range current.NumFields() {
			collectTypeImportsInto(manager, current.Field(index).Type(), seen)
		}
	case *types.Tuple:
		collectTupleImports(manager, current, seen)
	}
}

func collectTupleImports(manager *importManager, values *types.Tuple, seen map[types.Type]bool) {
	if values == nil {
		return
	}
	for index := range values.Len() {
		collectTypeImportsInto(manager, values.At(index).Type(), seen)
	}
}
