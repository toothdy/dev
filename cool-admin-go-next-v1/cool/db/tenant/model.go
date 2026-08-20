package tenant

import (
	"context"
	"database/sql"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

// ModelProvider 表示可创建 GoFrame Model 的数据库或事务
type ModelProvider interface {
	Model(tableNameQueryOrStruct ...interface{}) *gdb.Model
}

// modelScope 保存单次 Model 已解析的租户计划
type modelScope struct {
	metadata Metadata
	scope    Scope
}

/**
 * 创建带租户作用域的 GoFrame Model
 * @param ctx 上下文
 * @param provider 数据库或事务
 * @param definition 模型定义
 * @param alias 可选表别名
 * @returns Scoped Model 和校验错误
 */
func ScopedModel(ctx context.Context, provider ModelProvider, definition entity.Definition, alias string) (*gdb.Model, error) {
	if provider == nil {
		return nil, exception.Core("Scoped Model 缺少数据库提供者")
	}
	if definition.TableName == "" {
		return nil, exception.Core("Scoped Model 缺少数据表")
	}
	if alias != "" && !isIdentifier(alias) {
		return nil, exception.Validate("租户查询别名无效")
	}
	metadata, err := CompileMetadata(definition)
	if err != nil {
		return nil, exception.Core(err.Error())
	}
	scope := Resolve(ctx)
	if err = ValidateScope(scope, metadata); err != nil {
		return nil, err
	}

	dbModel := provider.Model(definition.TableName).Ctx(ctx)
	if alias != "" {
		dbModel = dbModel.As(alias)
	}
	if !metadata.IsAware() {
		return dbModel, nil
	}
	condition, err := PredicateForScope(scope, metadata, alias)
	if err != nil {
		return nil, err
	}
	if condition.SQL != "" {
		dbModel = dbModel.Where(condition.SQL, condition.Args...)
	}
	plan := modelScope{metadata: metadata, scope: scope}
	return dbModel.Hook(plan.hooks()), nil
}

/**
 * 创建租户写操作 Hook
 * @returns GoFrame Hook 集合
 */
func (p modelScope) hooks() gdb.HookHandler {
	return gdb.HookHandler{
		Insert: p.insert,
		Update: p.update,
		Delete: p.delete,
	}
}

/**
 * 覆盖新增数据的租户值
 * @param ctx 上下文
 * @param input GoFrame Insert Hook 输入
 * @returns SQL 结果和错误
 */
func (p modelScope) insert(ctx context.Context, input *gdb.HookInsertInput) (sql.Result, error) {
	value, shouldWrite, err := InsertValue(p.scope, p.metadata)
	if err != nil {
		return nil, err
	}
	if shouldWrite {
		rows := make(gdb.List, len(input.Data))
		for index, row := range input.Data {
			cloned := make(gdb.Map, len(row)+1)
			for key, item := range row {
				if strings.EqualFold(key, p.metadata.JSONField()) || containsSQLIdentifier(key, p.metadata.Column()) {
					continue
				}
				cloned[key] = item
			}
			cloned[p.metadata.Column()] = value
			rows[index] = cloned
		}
		input.Data = rows
	}
	return input.Next(ctx)
}

/**
 * 防止更新租户列并追加租户条件
 * @param ctx 上下文
 * @param input GoFrame Update Hook 输入
 * @returns SQL 结果和错误
 */
func (p modelScope) update(ctx context.Context, input *gdb.HookUpdateInput) (sql.Result, error) {
	data, err := sanitizeHookUpdateData(input.Data, p.metadata.Column())
	if err != nil {
		return nil, err
	}
	input.Data = data
	if err = p.appendMutationPredicate(&input.Condition, &input.Args, updateDataArgumentCount(data)); err != nil {
		return nil, err
	}
	return input.Next(ctx)
}

/**
 * 追加删除操作的租户条件
 * @param ctx 上下文
 * @param input GoFrame Delete Hook 输入
 * @returns SQL 结果和错误
 */
func (p modelScope) delete(ctx context.Context, input *gdb.HookDeleteInput) (sql.Result, error) {
	if err := p.appendMutationPredicate(&input.Condition, &input.Args, 0); err != nil {
		return nil, err
	}
	return input.Next(ctx)
}

/**
 * 追加写操作租户谓词
 * @param condition GoFrame 写入条件
 * @param args GoFrame 写入参数
 * @param dataArgumentCount SET 表达式占位符数量
 * @returns error
 */
func (p modelScope) appendMutationPredicate(condition *string, args *[]interface{}, dataArgumentCount int) error {
	predicate, err := PredicateForScope(p.scope, p.metadata, "")
	if err != nil {
		return err
	}
	if predicate.SQL == "" {
		return nil
	}
	trimmed := strings.TrimSpace(*condition)
	if trimmed == "" {
		*condition = predicate.SQL
	} else {
		*condition = predicate.SQL + " AND " + trimmed
	}
	if dataArgumentCount < 0 || dataArgumentCount > len(*args) {
		return exception.Core("GoFrame Update Hook 参数数量无效")
	}
	values := make([]interface{}, 0, len(*args)+len(predicate.Args))
	values = append(values, (*args)[:dataArgumentCount]...)
	values = append(values, predicate.Args...)
	values = append(values, (*args)[dataArgumentCount:]...)
	*args = values
	return nil
}

/**
 * 统计 Update SET 表达式的参数数量
 * @param data GoFrame 标准化更新数据
 * @returns SET 占位符数量
 */
func updateDataArgumentCount(data interface{}) int {
	value, ok := data.(string)
	if !ok {
		return 0
	}
	return countSQLPlaceholders(value)
}

/**
 * 净化 GoFrame 标准化后的更新数据
 * @param data 更新数据
 * @param tenantColumn 租户列名
 * @returns 净化数据和错误
 */
func sanitizeHookUpdateData(data interface{}, tenantColumn string) (interface{}, error) {
	switch values := data.(type) {
	case map[string]interface{}:
		cloned := make(map[string]interface{}, len(values))
		for key, value := range values {
			if containsSQLIdentifier(key, tenantColumn) {
				continue
			}
			cloned[key] = value
		}
		return cloned, nil
	case string:
		if containsSQLIdentifier(values, tenantColumn) {
			return nil, exception.Validate("租户字段不允许修改")
		}
		return values, nil
	default:
		return nil, exception.Core("GoFrame Update Hook 数据类型无效")
	}
}

/**
 * 检查 SQL 片段是否包含指定标识符
 * @param value SQL 片段
 * @param identifier 标识符
 * @returns 是否包含
 */
func containsSQLIdentifier(value string, identifier string) bool {
	for start := 0; start < len(value); {
		for start < len(value) && !isIdentifierByte(value[start]) {
			start++
		}
		end := start
		for end < len(value) && isIdentifierByte(value[end]) {
			end++
		}
		if end > start && strings.EqualFold(value[start:end], identifier) {
			return true
		}
		start = end
	}
	return false
}

/**
 * 统计 SQL 片段中的参数占位符
 * @param value SQL 片段
 * @returns 占位符数量
 */
func countSQLPlaceholders(value string) int {
	var (
		count   int
		quote   byte
		escaped bool
	)
	for index := 0; index < len(value); index++ {
		item := value[index]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if item == '\\' {
				escaped = true
				continue
			}
			if item == quote {
				if index+1 < len(value) && value[index+1] == quote {
					index++
					continue
				}
				quote = 0
			}
			continue
		}
		if item == '\'' || item == '"' || item == '`' {
			quote = item
			continue
		}
		if item == '?' {
			count++
		}
	}
	return count
}

/**
 * 判断字节是否属于 SQL 标识符
 * @param value 字节
 * @returns 是否属于
 */
func isIdentifierByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_'
}
