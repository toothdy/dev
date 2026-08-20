package codegen

import (
	"fmt"
	"go/types"
	"strings"
)

const (
	modelPackagePath      = "github.com/toothdy/cool-admin-go-next/cool/entity"
	controllerPackagePath = "github.com/toothdy/cool-admin-go-next/cool/controller"
	middlewarePackagePath = "github.com/toothdy/cool-admin-go-next/cool/middleware"
	taskPackagePath       = "github.com/toothdy/cool-admin-go-next/cool/task"
	registryPackagePath   = "github.com/toothdy/cool-admin-go-next/cool/module"
)

// functionSignature 读取函数的完整类型签名。
func functionSignature(function SourceFunction) (*types.Signature, error) {
	object := function.Package.TypesInfo.Defs[function.Decl.Name]
	if object == nil {
		return nil, fmt.Errorf("%s: 无法读取函数 %s 的类型信息", function.Pos, function.Decl.Name.Name)
	}
	signature, ok := object.Type().(*types.Signature)
	if !ok {
		return nil, fmt.Errorf("%s: %s 不是函数签名", function.Pos, function.Decl.Name.Name)
	}
	return signature, nil
}

func namedType(typ types.Type, packagePath string, name string) bool {
	typ = types.Unalias(typ)
	named, ok := typ.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == packagePath && named.Obj().Name() == name
}

func sliceOfNamedType(typ types.Type, packagePath string, name string) bool {
	slice, ok := typ.(*types.Slice)
	return ok && namedType(slice.Elem(), packagePath, name)
}

func resultTypeName(typ types.Type) string {
	typ = types.Unalias(typ)
	for {
		switch current := typ.(type) {
		case *types.Pointer:
			typ = types.Unalias(current.Elem())
		default:
			named, ok := typ.(*types.Named)
			if !ok || named.Obj() == nil {
				return ""
			}
			return named.Obj().Name()
		}
	}
}

func typeID(typ types.Type) string {
	return types.TypeString(typ, func(pkg *types.Package) string {
		if pkg == nil {
			return ""
		}
		return pkg.Path()
	})
}

func isErrorType(typ types.Type) bool {
	return types.Identical(typ, types.Universe.Lookup("error").Type())
}

func isPrimitiveDependency(typ types.Type) bool {
	typ = types.Unalias(typ)
	switch current := typ.(type) {
	case *types.Basic:
		return true
	case *types.Slice:
		_, ok := types.Unalias(current.Elem()).(*types.Basic)
		return ok
	}
	return false
}

func relativeDirectory(path string) string {
	if index := strings.IndexByte(path, '/'); index >= 0 {
		return path[:index]
	}
	return ""
}
