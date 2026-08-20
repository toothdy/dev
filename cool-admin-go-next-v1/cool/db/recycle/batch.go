package recycle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

const MaxBatchItems = 10000

// IdentityField 表示数据身份的一个有序字段
type IdentityField struct {
	JSONName   string          `json:"jsonName"`
	ColumnName string          `json:"columnName"`
	Value      json.RawMessage `json:"value"`
}

// Identity 表示单主键或联合唯一身份
type Identity struct {
	Fields []IdentityField `json:"fields"`
}

// ItemOptions 表示归档项的依赖配置
type ItemOptions struct {
	BranchKey      string
	ParentKey      string
	RestoreOrder   int
	IdentityFields []string
}

// Batch 表示一次事务删除的归档批次
type Batch struct {
	catalog  *Catalog
	archive  *Archive
	items    map[string]*ArchiveItem
	nextKey  int
	rootKeys map[string]string
}

/**
 * 创建删除归档批次
 * @param catalog 冻结模型目录
 * @param archive 归档主记录
 * @returns *Batch
 */
func NewBatch(catalog *Catalog, archive *Archive) *Batch {
	if archive == nil {
		archive = &Archive{}
	}
	return &Batch{
		catalog:  catalog,
		archive:  archive,
		items:    map[string]*ArchiveItem{},
		rootKeys: map[string]string{},
	}
}

/**
 * 追加一条归档快照
 * @param definition 模型定义
 * @param record 数据记录
 * @param options 依赖配置
 * @returns 归档项内部键和错误
 */
func (b *Batch) AddRecord(definition entity.Definition, record map[string]interface{}, options ItemOptions) (string, error) {
	if b == nil || b.catalog == nil || b.archive == nil {
		return "", gerror.New("回收站批次未初始化")
	}
	if len(b.items) >= MaxBatchItems {
		return "", gerror.Newf("单次回收数据不能超过 %d 条", MaxBatchItems)
	}
	metadata, ok := b.catalog.ModelByTable(definition.TableName)
	if !ok || metadata.Resource != definition.ResourceKey() {
		return "", gerror.Newf("回收站模型未注册: %s", definition.ResourceKey())
	}
	normalized, err := normalizeRecord(metadata, record)
	if err != nil {
		return "", err
	}
	identityFields, err := resolveIdentityFields(metadata, options.IdentityFields)
	if err != nil {
		return "", err
	}
	identity, err := buildIdentity(identityFields, normalized)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", gerror.Wrap(err, "序列化回收站数据快照失败")
	}
	branchKey := strings.TrimSpace(options.BranchKey)
	if branchKey == "" {
		branchKey, err = identity.CanonicalKey()
		if err != nil {
			return "", err
		}
	}
	b.nextKey++
	key := fmt.Sprintf("item-%d", b.nextKey)
	item := &ArchiveItem{
		Key:          key,
		Resource:     metadata.Resource,
		TableName:    definition.TableName,
		Identity:     identity,
		Data:         data,
		BranchKey:    branchKey,
		ParentKey:    options.ParentKey,
		RestoreOrder: options.RestoreOrder,
		Status:       ItemStatusPending,
		TenantID:     cloneInt64Pointer(b.archive.TenantID),
	}
	b.items[key] = item
	b.archive.Items = append(b.archive.Items, item)
	return key, nil
}

/**
 * 追加 GoFrame 查询结果
 * @param definition 模型定义
 * @param rows 数据记录
 * @param options 依赖配置
 * @returns 归档项内部键和错误
 */
func (b *Batch) AddResult(definition entity.Definition, rows gdb.Result, options ItemOptions) ([]string, error) {
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		key, err := b.AddRecord(definition, row.Map(), options)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

/**
 * 记录根身份与归档项的对应关系
 * @param value 根身份值
 * @param itemKey 归档项内部键
 * @returns error
 */
func (b *Batch) SetRootKey(value interface{}, itemKey string) error {
	canonical, err := canonicalValue(value)
	if err != nil {
		return err
	}
	if _, ok := b.items[itemKey]; !ok {
		return gerror.Newf("回收站根归档项不存在: %s", itemKey)
	}
	b.rootKeys[canonical] = itemKey
	return nil
}

/**
 * 按根身份值查找归档项内部键
 * @param value 根身份值
 * @returns 归档项内部键和是否存在
 */
func (b *Batch) RootKey(value interface{}) (string, bool) {
	canonical, err := canonicalValue(value)
	if err != nil {
		return "", false
	}
	key, ok := b.rootKeys[canonical]
	return key, ok
}

/**
 * 校验并稳定排序归档依赖图
 * @returns error
 */
func (b *Batch) Validate() error {
	if b == nil || b.archive == nil {
		return gerror.New("回收站批次未初始化")
	}
	if len(b.items) == 0 {
		return gerror.New("回收站批次不能为空")
	}
	if len(b.items) > MaxBatchItems {
		return gerror.Newf("单次回收数据不能超过 %d 条", MaxBatchItems)
	}
	state := make(map[string]uint8, len(b.items))
	depth := make(map[string]int, len(b.items))
	var visit func(string) (int, error)
	visit = func(key string) (int, error) {
		switch state[key] {
		case 1:
			return 0, gerror.New("回收站归档依赖不能成环")
		case 2:
			return depth[key], nil
		}
		item, ok := b.items[key]
		if !ok {
			return 0, gerror.Newf("回收站归档项不存在: %s", key)
		}
		state[key] = 1
		itemDepth := 0
		if item.ParentKey != "" {
			parent, parentExists := b.items[item.ParentKey]
			if !parentExists {
				return 0, gerror.Newf("回收站父归档项不存在: %s", item.ParentKey)
			}
			if parent.BranchKey != item.BranchKey {
				return 0, gerror.New("回收站归档项不能跨分支依赖")
			}
			parentDepth, err := visit(item.ParentKey)
			if err != nil {
				return 0, err
			}
			itemDepth = parentDepth + 1
		}
		state[key] = 2
		depth[key] = itemDepth
		return itemDepth, nil
	}
	for key := range b.items {
		if _, err := visit(key); err != nil {
			return err
		}
	}
	sort.SliceStable(b.archive.Items, func(left int, right int) bool {
		leftItem := b.archive.Items[left]
		rightItem := b.archive.Items[right]
		if depth[leftItem.Key] != depth[rightItem.Key] {
			return depth[leftItem.Key] < depth[rightItem.Key]
		}
		if leftItem.RestoreOrder != rightItem.RestoreOrder {
			return leftItem.RestoreOrder < rightItem.RestoreOrder
		}
		if leftItem.BranchKey != rightItem.BranchKey {
			return leftItem.BranchKey < rightItem.BranchKey
		}
		return leftItem.Key < rightItem.Key
	})
	b.archive.Count = len(b.archive.Items)
	b.archive.RemainingCount = len(b.archive.Items)
	b.archive.RestoreStatus = RestoreStatusPending
	return nil
}

/**
 * 返回当前归档主记录
 * @returns *Archive
 */
func (b *Batch) Archive() *Archive {
	if b == nil {
		return nil
	}
	return b.archive
}

/**
 * 返回身份的稳定比较键
 * @returns string 和错误
 */
func (i Identity) CanonicalKey() (string, error) {
	if len(i.Fields) == 0 {
		return "", gerror.New("回收站数据身份不能为空")
	}
	content, err := json.Marshal(i)
	if err != nil {
		return "", gerror.Wrap(err, "序列化回收站数据身份失败")
	}
	return string(content), nil
}

func resolveIdentityFields(metadata ModelMetadata, names []string) ([]entity.Field, error) {
	if len(names) == 0 {
		if len(metadata.IdentityFields) == 0 {
			return nil, gerror.Newf("回收站模型 %s 缺少稳定数据身份", metadata.Resource)
		}
		return append([]entity.Field{}, metadata.IdentityFields...), nil
	}
	if len(metadata.IdentityFields) > 0 {
		if len(names) != len(metadata.IdentityFields) {
			return nil, gerror.Newf("回收站模型 %s 不允许覆盖已编译数据身份", metadata.Resource)
		}
		for index, field := range metadata.IdentityFields {
			if names[index] != field.JSONName && names[index] != field.ColumnName {
				return nil, gerror.Newf("回收站模型 %s 不允许覆盖已编译数据身份", metadata.Resource)
			}
		}
		return append([]entity.Field{}, metadata.IdentityFields...), nil
	}
	fields := make([]entity.Field, 0, len(names))
	seen := map[string]struct{}{}
	for _, name := range names {
		field, ok := metadata.FieldsByJSON[name]
		if !ok {
			field, ok = metadata.FieldsByColumn[name]
		}
		if !ok {
			return nil, gerror.Newf("回收站身份字段不存在: %s.%s", metadata.Resource, name)
		}
		if _, exists := seen[field.JSONName]; exists {
			return nil, gerror.Newf("回收站身份字段重复: %s.%s", metadata.Resource, name)
		}
		seen[field.JSONName] = struct{}{}
		fields = append(fields, field)
	}
	return fields, nil
}

func buildIdentity(fields []entity.Field, record map[string]interface{}) (Identity, error) {
	identity := Identity{Fields: make([]IdentityField, 0, len(fields))}
	for _, field := range fields {
		value, ok := record[field.JSONName]
		if !ok || value == nil {
			return Identity{}, gerror.Newf("回收站数据缺少身份字段: %s", field.JSONName)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return Identity{}, gerror.Wrapf(err, "序列化回收站身份字段 %s 失败", field.JSONName)
		}
		identity.Fields = append(identity.Fields, IdentityField{
			JSONName: field.JSONName, ColumnName: field.ColumnName, Value: encoded,
		})
	}
	return identity, nil
}

func normalizeRecord(metadata ModelMetadata, record map[string]interface{}) (map[string]interface{}, error) {
	normalized := make(map[string]interface{}, len(record))
	for name, value := range record {
		field, ok := metadata.FieldsByJSON[name]
		if !ok {
			field, ok = metadata.FieldsByColumn[name]
		}
		if !ok {
			return nil, gerror.Newf("回收站模型 %s 存在未知字段: %s", metadata.Resource, name)
		}
		normalized[field.JSONName] = normalizeSnapshotValue(field, value)
	}
	for _, field := range metadata.Definition.FieldsValue {
		if _, ok := normalized[field.JSONName]; !ok {
			return nil, gerror.Newf("回收站模型 %s 快照缺少字段: %s", metadata.Resource, field.JSONName)
		}
	}
	return normalized, nil
}

func normalizeSnapshotValue(field entity.Field, value interface{}) interface{} {
	if value == nil {
		return nil
	}
	isJSON := strings.EqualFold(field.DataType, "json")
	switch typed := value.(type) {
	case []byte:
		cloned := append([]byte{}, typed...)
		if isJSON && json.Valid(cloned) {
			return json.RawMessage(cloned)
		}
		return string(cloned)
	case string:
		if isJSON && json.Valid([]byte(typed)) {
			return json.RawMessage(append([]byte{}, typed...))
		}
	}
	return value
}

func canonicalValue(value interface{}) (string, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return "", gerror.Wrap(err, "序列化回收站根身份失败")
	}
	var compact bytes.Buffer
	if err = json.Compact(&compact, content); err != nil {
		return "", gerror.Wrap(err, "压缩回收站根身份失败")
	}
	return compact.String(), nil
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
