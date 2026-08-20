package tenant

import (
	"context"
	"fmt"

	"github.com/toothdy/cool-admin-go-next/cool/exception"
)

// Condition 表示参数化租户查询条件
type Condition struct {
	SQL  string
	Args []interface{}
}

/**
 * 构建资源租户谓词
 * @param ctx 请求上下文
 * @param metadata 租户元数据
 * @param alias 可选表别名
 * @returns 参数化条件和作用域错误
 */
func Predicate(ctx context.Context, metadata Metadata, alias string) (Condition, error) {
	return PredicateForScope(Resolve(ctx), metadata, alias)
}

/**
 * 使用已解析作用域构建租户谓词
 * @param scope 租户作用域
 * @param metadata 租户元数据
 * @param alias 可选表别名
 * @returns 参数化条件和作用域错误
 */
func PredicateForScope(scope Scope, metadata Metadata, alias string) (Condition, error) {
	if !metadata.IsAware() {
		return Condition{}, nil
	}
	switch scope.Kind() {
	case KindPlatform, KindBypass:
		return Condition{}, nil
	case KindTenant:
		if alias != "" && !isIdentifier(alias) {
			return Condition{}, exception.Validate("租户查询别名无效")
		}
		column := quoteIdentifier(metadata.Column())
		if alias != "" {
			column = quoteIdentifier(alias) + "." + column
		}
		tenantID, _ := scope.TenantID()
		return Condition{SQL: column + " = ?", Args: []interface{}{tenantID}}, nil
	default:
		return Condition{}, exception.Unauthorized()
	}
}

/**
 * 构建只读取平台数据的公开查询谓词
 * @param metadata 租户元数据
 * @param alias 可选表别名
 * @returns 平台数据条件和配置错误
 */
func GlobalOnlyPredicate(metadata Metadata, alias string) (Condition, error) {
	if !metadata.IsAware() {
		return Condition{}, nil
	}
	if alias != "" && !isIdentifier(alias) {
		return Condition{}, exception.Validate("租户查询别名无效")
	}
	column := quoteIdentifier(metadata.Column())
	if alias != "" {
		column = quoteIdentifier(alias) + "." + column
	}
	return Condition{SQL: column + " IS NULL"}, nil
}

/**
 * 获取新增记录的租户值
 * @param scope 租户作用域
 * @param metadata 租户元数据
 * @returns 租户值、是否写入和作用域错误
 */
func InsertValue(scope Scope, metadata Metadata) (interface{}, bool, error) {
	if !metadata.IsAware() {
		return nil, false, nil
	}
	switch scope.Kind() {
	case KindPlatform, KindBypass:
		return nil, true, nil
	case KindTenant:
		tenantID, _ := scope.TenantID()
		return tenantID, true, nil
	default:
		return nil, false, exception.Unauthorized()
	}
}

/**
 * 校验资源租户作用域
 * @param scope 租户作用域
 * @param metadata 租户元数据
 * @returns 作用域错误
 */
func ValidateScope(scope Scope, metadata Metadata) error {
	if !metadata.IsAware() || scope.Kind() != KindMissing {
		return nil
	}
	return exception.Unauthorized()
}

/**
 * 校验 SQL 标识符
 * @param value 标识符
 * @returns 是否有效
 */
func isIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, item := range []byte(value) {
		isLetter := item >= 'a' && item <= 'z' || item >= 'A' && item <= 'Z'
		isDigit := item >= '0' && item <= '9'
		if !(isLetter || item == '_' || index > 0 && isDigit) {
			return false
		}
	}
	return true
}

/**
 * 引用 SQL 标识符
 * @param value 标识符
 * @returns 引用后的标识符
 */
func quoteIdentifier(value string) string {
	return fmt.Sprintf("`%s`", value)
}
