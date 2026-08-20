package seed

import (
	"encoding/json"
	"os"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

/**
 * 解析 DB seed 内容
 * @param data 文件内容
 * @param models 模型映射
 * @returns 映射记录列表
 */
func ParseDBContent(data []byte, models ModelMap) ([]MappedRecord, error) {
	var groups map[string][]RawRecord
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, gerror.Wrap(err, "解析 DB seed JSON 失败")
	}

	records := make([]MappedRecord, 0)
	for tableName, items := range groups {
		if _, ok := models[tableName]; !ok {
			return nil, gerror.Newf("未知 DB seed 表: %s", tableName)
		}
		for _, item := range items {
			if err := appendDBRecord(&records, models, tableName, item, nil); err != nil {
				return nil, err
			}
		}
	}
	return records, nil
}

/**
 * 解析菜单 seed 内容
 * @param data 文件内容
 * @param menuDefinition 菜单模型定义
 * @returns 映射记录列表
 */
func ParseMenuContent(data []byte, menuDefinition entity.Definition) ([]MappedRecord, error) {
	var items []RawRecord
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, gerror.Wrap(err, "解析菜单 seed JSON 失败")
	}

	models := NewModelMap([]entity.Definition{menuDefinition})
	records := make([]MappedRecord, 0)
	for _, item := range items {
		if err := appendMenuRecord(&records, models, menuDefinition.TableName, item, -1, nil); err != nil {
			return nil, err
		}
	}
	return records, nil
}

/**
 * 加载 DB seed 文件
 * @param path 文件路径
 * @param models 模型映射
 * @returns 映射记录列表
 */
func LoadDBFile(path string, models ModelMap) ([]MappedRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, gerror.Wrapf(err, "读取 DB seed 文件失败: %s", path)
	}
	return ParseDBContent(data, models)
}

/**
 * 加载菜单 seed 文件
 * @param path 文件路径
 * @param menuDefinition 菜单模型定义
 * @returns 映射记录列表
 */
func LoadMenuFile(path string, menuDefinition entity.Definition) ([]MappedRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, gerror.Wrapf(err, "读取菜单 seed 文件失败: %s", path)
	}
	return ParseMenuContent(data, menuDefinition)
}

/**
 * 追加 DB 记录
 * @param records 记录列表
 * @param models 模型映射
 * @param tableName 表名
 * @param item 原始记录
 * @param parent 父级记录
 * @returns error
 */
func appendDBRecord(records *[]MappedRecord, models ModelMap, tableName string, item RawRecord, parent RawRecord) error {
	mapped, err := MapRecord(models, tableName, item, parent)
	if err != nil {
		return err
	}
	*records = append(*records, mapped)

	childDatas, ok := item[childDatasKey]
	if !ok || childDatas == nil {
		return nil
	}

	groups, ok := childDatas.(map[string]interface{})
	if !ok {
		return gerror.New("@childDatas 必须是对象")
	}
	for childTableName, rawItems := range groups {
		items, ok := rawItems.([]interface{})
		if !ok {
			return gerror.Newf("@childDatas.%s 必须是数组", childTableName)
		}
		for _, rawItem := range items {
			childItem, ok := rawItem.(map[string]interface{})
			if !ok {
				return gerror.Newf("@childDatas.%s 子项必须是对象", childTableName)
			}
			if err := appendDBRecord(records, models, childTableName, RawRecord(childItem), item); err != nil {
				return err
			}
		}
	}
	return nil
}

/**
 * 追加菜单记录
 * @param records 记录列表
 * @param models 模型映射
 * @param tableName 表名
 * @param item 原始记录
 * @param parentIndex 父记录索引
 * @returns error
 */
func appendMenuRecord(records *[]MappedRecord, models ModelMap, tableName string, item RawRecord, parentIndex int, parent RawRecord) error {
	mapped, err := MapRecord(models, tableName, item, parent)
	if err != nil {
		return err
	}
	if parentIndex >= 0 {
		mapped.ParentIndex = parentIndex
		mapped.ParentColumn = "parentId"
	}

	currentIndex := len(*records)
	*records = append(*records, mapped)

	childMenus, ok := item[childMenusKey]
	if !ok || childMenus == nil {
		return nil
	}

	items, ok := childMenus.([]interface{})
	if !ok {
		return gerror.New("childMenus 必须是数组")
	}
	for _, rawItem := range items {
		childItem, ok := rawItem.(map[string]interface{})
		if !ok {
			return gerror.New("childMenus 子项必须是对象")
		}
		if err := appendMenuRecord(records, models, tableName, RawRecord(childItem), currentIndex, item); err != nil {
			return err
		}
	}
	return nil
}
