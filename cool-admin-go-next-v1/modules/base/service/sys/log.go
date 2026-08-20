package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/service"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
)

const logCleanupBatchSize = 1000

// 操作日志写入参数
type LogRecordRequest struct {
	UserID   *int64
	Action   string
	IP       string
	Params   string
	TenantID int64
}

type logRecordDO struct {
	UserID     interface{} `orm:"userId"`
	Action     interface{} `orm:"action"`
	IP         interface{} `orm:"ip"`
	Params     interface{} `orm:"params"`
	TenantID   interface{} `orm:"tenantId"`
	CreateTime interface{} `orm:"createTime"`
	UpdateTime interface{} `orm:"updateTime"`
}

// 操作日志服务
type LogService struct {
	*service.Base
	userModel entity.Definition
	confModel entity.Definition
}

/**
 * 创建操作日志服务
 * @param db 数据库实例
 * @param baseSysLogModel 操作日志模型
 * @param baseSysUserModel 用户模型
 * @param baseSysConfModel 系统配置模型
 * @returns *LogService
 */
func NewLogService(
	db gdb.DB,
	baseSysLogModel entity.Definition,
	baseSysUserModel entity.Definition,
	baseSysConfModel entity.Definition,
) *LogService {
	return &LogService{
		Base: service.NewBase(db, baseSysLogModel),
		userModel:   baseSysUserModel,
		confModel:   baseSysConfModel,
	}
}

// 写入一条操作日志
func (s *LogService) Record(ctx context.Context, request LogRecordRequest) error {
	if s == nil || s.Base == nil || s.DB == nil {
		return exception.Internal(nil, "操作日志服务未初始化")
	}
	var userID interface{}
	if request.UserID != nil {
		userID = *request.UserID
	}
	data := logRecordDO{
		UserID: userID,
		Action: request.Action,
		IP:     request.IP,
		Params: request.Params,
	}
	now := mutationTimestamp()
	data.CreateTime = now
	data.UpdateTime = now
	recordCtx := ctx
	if tenant.Resolve(ctx).Kind() == tenant.KindMissing {
		recordCtx = tenant.WithoutTenant(ctx)
	}
	dbModel, err := tenant.ScopedModel(recordCtx, s.DB, s.Model, "")
	if err != nil {
		return err
	}
	if _, err = dbModel.Data(data).Insert(); err != nil {
		return gerror.Wrap(err, "写入操作日志失败")
	}
	return nil
}

// 联表返回操作用户名称
func (s *LogService) Page(ctx context.Context, request crud.QueryRequest) (interface{}, error) {
	request = crud.NormalizePageRequest(request)
	logCondition, err := s.logTenantCondition(ctx, s.Model, "a")
	if err != nil {
		return nil, err
	}
	userCondition, err := s.logTenantCondition(ctx, s.userModel, "b")
	if err != nil {
		return nil, err
	}
	join := " LEFT JOIN base_sys_user b ON a.userId = b.id"
	joinArgs := []interface{}{}
	if userCondition.SQL != "" {
		join += " AND " + userCondition.SQL
		joinArgs = append(joinArgs, userCondition.Args...)
	}
	where := " WHERE 1 = 1"
	whereArgs := []interface{}{}
	if logCondition.SQL != "" {
		where += " AND " + logCondition.SQL
		whereArgs = append(whereArgs, logCondition.Args...)
	}
	if request.Keyword != "" {
		where += " AND (b.name LIKE ? OR a.action LIKE ? OR a.ip LIKE ?)"
		keyword := "%" + request.Keyword + "%"
		whereArgs = append(whereArgs, keyword, keyword, keyword)
	}
	args := append(append([]interface{}{}, joinArgs...), whereArgs...)
	total, err := s.DB.GetCount(ctx, "SELECT COUNT(*) FROM base_sys_log a"+join+where, args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询日志总数失败")
	}
	orderBy, err := pageOrderBy(request, map[string]string{
		"id": "a.id", "createTime": "a.createTime", "updateTime": "a.updateTime", "userId": "a.userId",
	}, "id", "DESC")
	if err != nil {
		return nil, err
	}
	limitSQL, limitArgs := sqlPageLimit(request)
	listArgs := append(append([]interface{}{}, args...), limitArgs...)
	query := fmt.Sprintf("SELECT a.id, a.userId AS userId, a.action, a.ip, a.params, a.createTime AS createTime, a.updateTime AS updateTime, a.tenantId AS tenantId, b.name FROM base_sys_log a%s%s%s%s", join, where, orderBy, limitSQL)
	rows, err := s.DB.GetAll(ctx, query, listArgs...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询日志分页失败")
	}
	list := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		item := row.Map()
		item["params"] = normalizeLogParams(item["params"])
		list = append(list, item)
	}
	return crud.PageResult{List: list, Pagination: crud.Pagination{Page: request.Page, Size: request.Size, Total: total}}, nil
}

func normalizeLogParams(value interface{}) interface{} {
	var encoded string
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		encoded = typed
	case []byte:
		encoded = string(typed)
	default:
		return value
	}
	var decoded interface{}
	if json.Unmarshal([]byte(encoded), &decoded) != nil {
		return value
	}
	return decoded
}

/**
 * 清空操作日志
 * @param ctx 请求上下文
 * @returns 清空错误
 */
func (s *LogService) Clear(ctx context.Context) error {
	ctx = recycle.WithBypass(ctx)
	query, err := tenant.ScopedModel(ctx, s.DB, s.Model, "")
	if err != nil {
		return err
	}
	if _, err = query.Where("id IS NOT NULL").Delete(); err != nil {
		return gerror.Wrap(err, "清空日志失败")
	}
	return nil
}

/**
 * 清理超过保留期的操作日志
 * @param ctx 运行时上下文
 * @returns 删除数量和清理错误
 */
func (s *LogService) ClearExpired(ctx context.Context) (int64, error) {
	if s == nil || s.Base == nil || s.DB == nil {
		return 0, exception.Internal(nil, "操作日志服务未初始化")
	}
	maintenanceCtx := recycle.WithBypass(tenant.WithoutTenant(ctx))
	confQuery, err := tenant.ScopedModel(maintenanceCtx, s.DB, s.confModel, "")
	if err != nil {
		return 0, err
	}
	value, err := confQuery.Where("cKey", "logKeep").Value("cValue")
	if err != nil {
		return 0, gerror.Wrap(err, "读取操作日志保留天数失败")
	}
	if value == nil {
		return 0, gerror.New("操作日志保留天数未配置")
	}
	keepDays, err := parseLogKeepDays(value.String())
	if err != nil {
		return 0, err
	}
	return s.clearExpiredBefore(maintenanceCtx, logRetentionCutoff(time.Now(), keepDays))
}

func (s *LogService) clearExpiredBefore(ctx context.Context, before time.Time) (int64, error) {
	ctx = recycle.WithBypass(tenant.WithoutTenant(ctx))
	var deleted int64
	for {
		if err := ctx.Err(); err != nil {
			return deleted, gerror.Wrap(err, "清理过期操作日志已取消")
		}
		query, err := tenant.ScopedModel(ctx, s.DB, s.Model, "")
		if err != nil {
			return deleted, err
		}
		result, err := query.WhereLT("createTime", before.Format("2006-01-02 15:04:05")).Limit(logCleanupBatchSize).Delete()
		if err != nil {
			return deleted, gerror.Wrap(err, "清理过期操作日志失败")
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return deleted, gerror.Wrap(err, "读取操作日志清理结果失败")
		}
		deleted += affected
		if affected < logCleanupBatchSize {
			return deleted, nil
		}
	}
}

func parseLogKeepDays(value string) (int, error) {
	keepDays, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || keepDays <= 0 {
		return 0, gerror.Newf("操作日志保留天数无效: %q", value)
	}
	return keepDays, nil
}

func logRetentionCutoff(now time.Time, keepDays int) time.Time {
	localNow := now.In(time.Local)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	return today.AddDate(0, 0, -keepDays)
}

// 返回日志联表的租户条件
func (s *LogService) logTenantCondition(ctx context.Context, definition entity.Definition, alias string) (tenant.Condition, error) {
	metadata, err := tenant.CompileMetadata(definition)
	if err != nil {
		return tenant.Condition{}, err
	}
	return tenant.Predicate(ctx, metadata, alias)
}
