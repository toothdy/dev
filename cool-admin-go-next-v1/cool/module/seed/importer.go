package seed

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

const (
	menuTableName = "base_sys_menu"
	adminRoleID   = 1
)

// seed 导入结果
type ImportResult struct {
	ModuleName      string
	Kind            Kind
	InsertedRecords int
	Skipped         bool
	MarkerKey       string
}

// seed 导入器
type Importer struct {
	db          gdb.DB
	models      ModelMap
	definitions []entity.Definition
}

/**
 * 创建 seed 导入器
 * @param db GoFrame 数据库实例
 * @param definitions 模型定义列表
 * @returns *Importer
 */
func NewImporter(db gdb.DB, definitions []entity.Definition) *Importer {
	return &Importer{
		db:          db,
		models:      NewModelMap(definitions),
		definitions: append([]entity.Definition{}, definitions...),
	}
}

/**
 * 导入 DB seed
 * @param ctx 上下文
 * @param moduleName 模块名
 * @param path seed 文件路径
 * @returns ImportResult
 */
func (i *Importer) ImportDB(ctx context.Context, moduleName string, path string) (ImportResult, error) {
	markerKey := MarkerKey(KindDB, moduleName)
	result := ImportResult{
		ModuleName: moduleName,
		Kind:       KindDB,
		MarkerKey:  markerKey,
	}

	isExists, err := i.markerExists(ctx, markerKey)
	if err != nil {
		return result, gerror.Wrap(err, "检查 DB seed 初始化标记失败")
	}
	if isExists {
		result.Skipped = true
		return result, nil
	}

	records, err := LoadDBFile(path, i.models)
	if err != nil {
		return result, gerror.Wrap(err, "加载 DB seed 文件失败")
	}

	startedAt := time.Now()
	if err = i.db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		isExists, err := i.markerExistsTX(tx, markerKey)
		if err != nil {
			return gerror.Wrap(err, "事务内检查 DB seed 初始化标记失败")
		}
		if isExists {
			result.Skipped = true
			return nil
		}

		insertedIDs, err := insertRecords(tx, records)
		if err != nil {
			return gerror.Wrap(err, "插入 DB seed 记录失败")
		}
		if err = writeMarkerTX(tx, markerKey, time.Since(startedAt)); err != nil {
			return gerror.Wrap(err, "写入 DB seed 初始化标记失败")
		}

		result.InsertedRecords = len(insertedIDs)
		return nil
	}); err != nil {
		return result, err
	}

	return result, nil
}

/**
 * 导入菜单 seed
 * @param ctx 上下文
 * @param moduleName 模块名
 * @param path seed 文件路径
 * @returns ImportResult
 */
func (i *Importer) ImportMenu(ctx context.Context, moduleName string, path string) (ImportResult, error) {
	markerKey := MarkerKey(KindMenu, moduleName)
	result := ImportResult{
		ModuleName: moduleName,
		Kind:       KindMenu,
		MarkerKey:  markerKey,
	}

	isExists, err := i.markerExists(ctx, markerKey)
	if err != nil {
		return result, gerror.Wrap(err, "检查菜单 seed 初始化标记失败")
	}
	if isExists {
		result.Skipped = true
		return result, nil
	}

	menuDefinition, ok := i.models[menuTableName]
	if !ok {
		return result, gerror.New("未找到 base_sys_menu 模型定义")
	}
	records, err := LoadMenuFile(path, menuDefinition)
	if err != nil {
		return result, gerror.Wrap(err, "加载菜单 seed 文件失败")
	}

	startedAt := time.Now()
	if err = i.db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		isExists, err := i.markerExistsTX(tx, markerKey)
		if err != nil {
			return gerror.Wrap(err, "事务内检查菜单 seed 初始化标记失败")
		}
		if isExists {
			result.Skipped = true
			return nil
		}

		insertedIDs, err := insertRecords(tx, records)
		if err != nil {
			return gerror.Wrap(err, "插入菜单 seed 记录失败")
		}
		if err = grantMenusToAdmin(tx, insertedIDs); err != nil {
			return gerror.Wrap(err, "绑定 admin 角色菜单失败")
		}
		if err = writeMarkerTX(tx, markerKey, time.Since(startedAt)); err != nil {
			return gerror.Wrap(err, "写入菜单 seed 初始化标记失败")
		}

		result.InsertedRecords = len(insertedIDs)
		return nil
	}); err != nil {
		return result, err
	}

	return result, nil
}

/**
 * 生成插入 SQL
 * @param record 映射记录
 * @returns SQL 和参数
 */
func InsertSQL(record MappedRecord) (string, []interface{}) {
	columns := make([]string, 0, len(record.Values))
	for columnName := range record.Values {
		columns = append(columns, columnName)
	}
	sort.Strings(columns)

	quotedColumns := make([]string, 0, len(columns))
	placeholders := make([]string, 0, len(columns))
	args := make([]interface{}, 0, len(columns))
	for _, columnName := range columns {
		quotedColumns = append(quotedColumns, quoteSeedIdentifier(columnName))
		placeholders = append(placeholders, "?")
		args = append(args, record.Values[columnName])
	}

	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		quoteSeedIdentifier(record.TableName),
		strings.Join(quotedColumns, ", "),
		strings.Join(placeholders, ", "),
	), args
}

/**
 * 插入记录列表
 * @param tx 事务
 * @param records 记录列表
 * @returns 插入 ID 列表
 */
func insertRecords(tx gdb.TX, records []MappedRecord) ([]int64, error) {
	insertedIDs := make([]int64, 0, len(records))
	for index, record := range records {
		if record.ParentIndex >= 0 {
			if record.ParentIndex >= len(insertedIDs) {
				return nil, gerror.Newf("父级记录索引无效: %d", record.ParentIndex)
			}
			if _, ok := record.Values[record.ParentColumn]; !ok {
				record.Values[record.ParentColumn] = insertedIDs[record.ParentIndex]
			}
		}

		sqlText, args := InsertSQL(record)
		result, err := tx.Exec(sqlText, args...)
		if err != nil {
			return nil, gerror.Wrapf(err, "插入 seed 记录失败: table=%s index=%d", record.TableName, index)
		}
		insertedID, err := insertedRecordID(result, record)
		if err != nil {
			return nil, gerror.Wrapf(err, "获取 seed 插入 ID 失败: table=%s index=%d", record.TableName, index)
		}
		insertedIDs = append(insertedIDs, insertedID)
	}
	return insertedIDs, nil
}

/**
 * 获取插入记录 ID
 * @param result SQL 结果
 * @param record 映射记录
 * @returns int64
 */
func insertedRecordID(result sql.Result, record MappedRecord) (int64, error) {
	if value, ok := record.Values["id"]; ok {
		return numericID(value)
	}
	return result.LastInsertId()
}

/**
 * 转换数字 ID
 * @param value 原始值
 * @returns int64
 */
func numericID(value interface{}) (int64, error) {
	switch id := value.(type) {
	case int:
		return int64(id), nil
	case int64:
		return id, nil
	case float64:
		return int64(id), nil
	case string:
		var parsed int64
		if _, err := fmt.Sscan(id, &parsed); err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, gerror.Newf("不支持的 ID 类型: %T", value)
	}
}

/**
 * 初始化标记是否存在
 * @param ctx 上下文
 * @param markerKey 标记键
 * @returns bool
 */
func (i *Importer) markerExists(ctx context.Context, markerKey string) (bool, error) {
	count, err := i.db.GetCount(ctx, "SELECT COUNT(*) FROM `base_sys_conf` WHERE `cKey` = ?", markerKey)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

/**
 * 事务内初始化标记是否存在
 * @param tx 事务
 * @param markerKey 标记键
 * @returns bool
 */
func (i *Importer) markerExistsTX(tx gdb.TX, markerKey string) (bool, error) {
	count, err := tx.GetCount("SELECT COUNT(*) FROM `base_sys_conf` WHERE `cKey` = ?", markerKey)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

/**
 * 写入初始化标记
 * @param tx 事务
 * @param markerKey 标记键
 * @param elapsed 耗时
 * @returns error
 */
func writeMarkerTX(tx gdb.TX, markerKey string, elapsed time.Duration) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := tx.Exec(
		"INSERT INTO `base_sys_conf` (`cKey`, `cValue`, `createTime`, `updateTime`) VALUES (?, ?, ?, ?)",
		markerKey,
		fmt.Sprintf("time consuming: %s", elapsed.String()),
		now,
		now,
	)
	return err
}

/**
 * 绑定菜单给 admin 角色
 * @param tx 事务
 * @param menuIDs 菜单 ID 列表
 * @returns error
 */
func grantMenusToAdmin(tx gdb.TX, menuIDs []int64) error {
	for _, menuID := range menuIDs {
		if menuID <= 0 {
			continue
		}
		_, err := tx.Exec(
			"INSERT INTO `base_sys_role_menu` (`roleId`, `menuId`) VALUES (?, ?)",
			adminRoleID,
			menuID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

/**
 * 引用 SQL 标识符
 * @param name 标识符
 * @returns 已引用标识符
 */
func quoteSeedIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
