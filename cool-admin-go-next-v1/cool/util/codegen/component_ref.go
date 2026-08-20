package codegen

import (
	"fmt"
	"go/token"
	"path"
	"sort"
	"strings"
)

func resolveDeclaredMiddlewares(analysis *Analysis) error {
	rootImportPath := configImportPath(*analysis)
	discovered := make(map[string]Component)
	nonMiddlewares := make([]Component, 0, len(analysis.Components))
	for _, component := range analysis.Components {
		if component.Kind != ComponentMiddleware && component.Kind != ComponentGlobalMiddleware {
			nonMiddlewares = append(nonMiddlewares, component)
			continue
		}
		relative := strings.TrimPrefix(component.ImportPath, rootImportPath+"/")
		reference := relative + "#" + component.Function
		discovered[reference] = component
	}

	seen := make(map[string]struct{})
	ordered := make([]Component, 0, len(discovered))
	appendReferences := func(references []string, global bool) error {
		for _, reference := range references {
			packagePath, function, err := parseComponentReference(reference)
			if err != nil {
				return fmt.Errorf("模块 %s 组件引用 %q 格式无效: %w", analysis.Module.Key, reference, err)
			}
			canonical := packagePath + "#" + function
			if _, exists := seen[canonical]; exists {
				return fmt.Errorf("模块 %s 中间件引用重复: %s", analysis.Module.Key, canonical)
			}
			component, exists := discovered[canonical]
			if !exists {
				return fmt.Errorf("模块 %s 中间件引用不存在或签名无效: %s", analysis.Module.Key, canonical)
			}
			if global && component.Kind != ComponentGlobalMiddleware {
				return fmt.Errorf("模块 %s 中间件引用作用域错误: %s 只能用于 Middlewares", analysis.Module.Key, canonical)
			}
			if !global && component.Kind != ComponentMiddleware {
				return fmt.Errorf("模块 %s 中间件引用作用域错误: %s 只能用于 GlobalMiddlewares", analysis.Module.Key, canonical)
			}
			seen[canonical] = struct{}{}
			ordered = append(ordered, component)
		}
		return nil
	}
	if err := appendReferences(analysis.Declaration.Middlewares, false); err != nil {
		return err
	}
	if err := appendReferences(analysis.Declaration.GlobalMiddlewares, true); err != nil {
		return err
	}
	if len(seen) != len(discovered) {
		missing := make([]string, 0, len(discovered)-len(seen))
		for reference := range discovered {
			if _, exists := seen[reference]; !exists {
				missing = append(missing, reference)
			}
		}
		sort.Strings(missing)
		return fmt.Errorf("模块 %s 发现了未声明的中间件工厂: %s", analysis.Module.Key, strings.Join(missing, ", "))
	}
	analysis.Components = append(nonMiddlewares, ordered...)
	return nil
}

func parseComponentReference(reference string) (string, string, error) {
	if strings.Count(reference, "#") != 1 {
		return "", "", fmt.Errorf("必须使用 <相对包>#<导出函数>")
	}
	parts := strings.SplitN(reference, "#", 2)
	packagePath := parts[0]
	function := parts[1]
	if packagePath == "" || function == "" || path.Clean(packagePath) != packagePath || strings.HasPrefix(packagePath, "/") || strings.HasPrefix(packagePath, "../") || !token.IsExported(function) {
		return "", "", fmt.Errorf("必须使用规范相对包路径和导出函数")
	}
	return packagePath, function, nil
}
