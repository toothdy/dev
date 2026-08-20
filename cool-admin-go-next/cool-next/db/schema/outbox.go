package schema

import "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"

const (
	OutboxTableName = "cool_outbox"
	InboxTableName  = "cool_inbox"

	OutboxAvailableIndex = "idx_cool_outbox_available"
	OutboxLeaseIndex     = "idx_cool_outbox_lease"
	OutboxSentIndex      = "idx_cool_outbox_sent"
)

// 内部表字段定义
type DefinitionColumn struct {
	Name          string             // 列名
	Type          entity.LogicalType // 跨数据库逻辑类型
	Size          uint64             // 固定最大长度
	Nullable      bool               // 是否允许空值
	CaseSensitive bool               // 是否区分大小写
	CharacterSet  string             // 逻辑字符集
	Default       string             // 默认值或数据库表达式
	AllowedValues []string           // 有限字符串取值
}

// 内部表逻辑结构
type Definition struct {
	Name        string             // 表名
	Description string             // 表用途
	Columns     []DefinitionColumn // 有序列定义
	PrimaryKey  []string           // 有序主键列
	Indexes     []Index            // 有序固定索引
}

// 可靠发布记录表
func OutboxDefinition() Definition {
	return cloneDefinition(outboxDefinition)
}

// 消费幂等标记表
func InboxDefinition() Definition {
	return cloneDefinition(inboxDefinition)
}

var outboxDefinition = Definition{
	Name:        OutboxTableName,
	Description: "可靠消息发布记录",
	Columns: []DefinitionColumn{
		{Name: "messageId", Type: entity.LogicalString, Size: 36, CaseSensitive: true, CharacterSet: "ascii"},
		{Name: "topic", Type: entity.LogicalString},
		{Name: "messageType", Type: entity.LogicalString},
		{Name: "messageVersion", Type: entity.LogicalUint},
		{Name: "messageKey", Type: entity.LogicalString, Nullable: true},
		{Name: "payload", Type: entity.LogicalBytes},
		{Name: "headers", Type: entity.LogicalBytes},
		{Name: "status", Type: entity.LogicalString, Default: "pending", AllowedValues: []string{"pending", "retry", "leased", "sent", "dead"}},
		{Name: "attempts", Type: entity.LogicalUint, Default: "0"},
		{Name: "availableAt", Type: entity.LogicalTime, Default: "CURRENT_TIMESTAMP"},
		{Name: "leaseOwner", Type: entity.LogicalString, Nullable: true},
		{Name: "claimToken", Type: entity.LogicalString, Nullable: true},
		{Name: "leaseExpiresAt", Type: entity.LogicalTime, Nullable: true},
		{Name: "lastError", Type: entity.LogicalString, Nullable: true},
		{Name: "createTime", Type: entity.LogicalTime, Default: "CURRENT_TIMESTAMP"},
		{Name: "updateTime", Type: entity.LogicalTime, Default: "CURRENT_TIMESTAMP"},
		{Name: "sentAt", Type: entity.LogicalTime, Nullable: true},
	},
	PrimaryKey: []string{"messageId"},
	Indexes: []Index{
		{Name: OutboxAvailableIndex, Fields: []string{"status", "availableAt", "createTime", "messageId"}},
		{Name: OutboxLeaseIndex, Fields: []string{"status", "leaseExpiresAt", "messageId"}},
		{Name: OutboxSentIndex, Fields: []string{"status", "sentAt", "messageId"}},
	},
}

var inboxDefinition = Definition{
	Name:        InboxTableName,
	Description: "可靠消息消费幂等标记",
	Columns: []DefinitionColumn{
		{Name: "consumer", Type: entity.LogicalString, CaseSensitive: true},
		{Name: "messageId", Type: entity.LogicalString, Size: 36, CaseSensitive: true, CharacterSet: "ascii"},
		{Name: "processedAt", Type: entity.LogicalTime, Default: "CURRENT_TIMESTAMP"},
	},
	PrimaryKey: []string{"consumer", "messageId"},
}

func cloneDefinition(source Definition) Definition {
	result := source
	result.Columns = append([]DefinitionColumn(nil), source.Columns...)
	for index := range result.Columns {
		result.Columns[index].AllowedValues = append([]string(nil), source.Columns[index].AllowedValues...)
	}
	result.PrimaryKey = append([]string(nil), source.PrimaryKey...)
	result.Indexes = append([]Index(nil), source.Indexes...)
	for index := range result.Indexes {
		result.Indexes[index].Fields = append([]string(nil), source.Indexes[index].Fields...)
	}

	return result
}
