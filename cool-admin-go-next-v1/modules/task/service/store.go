package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	taskQueue "github.com/toothdy/cool-admin-go-next/modules/task/queue"
)

// ErrTaskLeaseLost 表示当前执行已不再持有任务租约。
var ErrTaskLeaseLost = errors.New("任务执行租约已丢失")

// TaskLease 描述一次领取成功的任务执行租约。
type TaskLease struct {
	Token     string
	ExpiresAt time.Time
}

// TaskInfo 是任务定义的持久化模型。
type TaskInfo struct {
	ID              int64       `orm:"id" json:"id"`
	CreateTime      string      `orm:"createTime" json:"createTime"`
	UpdateTime      string      `orm:"updateTime" json:"updateTime"`
	TenantID        *int64      `orm:"tenantId" json:"tenantId"`
	JobID           string      `orm:"jobId" json:"jobId"`
	RepeatConf      string      `orm:"repeatConf" json:"repeatConf"`
	Name            string      `orm:"name" json:"name"`
	Cron            string      `orm:"cron" json:"cron"`
	Limit           *int        `orm:"limit" json:"limit"`
	Every           *int64      `orm:"every" json:"every"`
	Remark          string      `orm:"remark" json:"remark"`
	Status          int         `orm:"status" json:"status"`
	StartDate       *gtime.Time `orm:"startDate" json:"startDate"`
	EndDate         *gtime.Time `orm:"endDate" json:"endDate"`
	Data            string      `orm:"data" json:"data"`
	Service         string      `orm:"service" json:"service"`
	Type            int         `orm:"type" json:"type"`
	NextRunTime     *gtime.Time `orm:"nextRunTime" json:"nextRunTime"`
	TaskType        int         `orm:"taskType" json:"taskType"`
	LastExecuteTime *gtime.Time `orm:"lastExecuteTime" json:"lastExecuteTime"`
	LockExpireTime  *gtime.Time `orm:"lockExpireTime" json:"lockExpireTime"`
	LockOwner       string      `orm:"lockOwner" json:"-"`
}

// TaskInfoDO 是任务定义的类型化写入对象。
type TaskInfoDO struct {
	g.Meta          `orm:"do:true"`
	JobID           interface{} `orm:"jobId"`
	RepeatConf      interface{} `orm:"repeatConf"`
	Name            interface{} `orm:"name"`
	Cron            interface{} `orm:"cron"`
	Limit           interface{} `orm:"limit"`
	Every           interface{} `orm:"every"`
	Remark          interface{} `orm:"remark"`
	Status          interface{} `orm:"status"`
	StartDate       interface{} `orm:"startDate"`
	EndDate         interface{} `orm:"endDate"`
	Data            interface{} `orm:"data"`
	Service         interface{} `orm:"service"`
	Type            interface{} `orm:"type"`
	NextRunTime     interface{} `orm:"nextRunTime"`
	TaskType        interface{} `orm:"taskType"`
	LastExecuteTime interface{} `orm:"lastExecuteTime"`
	LockExpireTime  interface{} `orm:"lockExpireTime"`
	LockOwner       interface{} `orm:"lockOwner"`
}

// RepeatState 保存当前调度世代的执行次数。
type RepeatState struct {
	Version    int    `json:"version"`
	Mode       string `json:"mode"`
	Generation string `json:"generation"`
	Count      int    `json:"count"`
}

type taskLogDO struct {
	g.Meta `orm:"do:true"`
	TaskID int64  `orm:"taskId"`
	Status int    `orm:"status"`
	Detail string `orm:"detail"`
}

type taskLogRecord struct {
	ID         int64  `orm:"id" json:"id"`
	CreateTime string `orm:"createTime" json:"createTime"`
	UpdateTime string `orm:"updateTime" json:"updateTime"`
	TenantID   *int64 `orm:"tenantId" json:"tenantId"`
	TaskID     int64  `orm:"taskId" json:"taskId"`
	Status     int    `orm:"status" json:"status"`
	Detail     string `orm:"detail" json:"detail"`
	TaskName   string `orm:"task_name" json:"taskName"`
}

// Store 封装 Task 的租户作用域和持久化操作。
type Store struct {
	db        gdb.DB
	infoModel entity.Definition
	logModel  entity.Definition
}

func taskTimeModel(query *gdb.Model) *gdb.Model {
	return query.SoftTime(gdb.SoftTimeOption{SoftTimeType: gdb.SoftTimeTypeTime})
}

// scheduledBatchTime 将计划批次键对齐到 MySQL datetime 的秒精度。
func scheduledBatchTime(value time.Time) time.Time {
	return value.Truncate(time.Second)
}

// BuildStore 创建 Task 持久化 Store。
func BuildStore(db gdb.DB, taskInfoModel entity.Definition, taskLogModel entity.Definition) (*Store, error) {
	if db == nil {
		return nil, gerror.New("Task 数据库不能为空")
	}
	if taskInfoModel.TableName == "" || taskLogModel.TableName == "" {
		return nil, gerror.New("Task 模型定义不完整")
	}
	return &Store{db: db, infoModel: taskInfoModel, logModel: taskLogModel}, nil
}

// Find 在当前租户作用域读取任务。
func (s *Store) Find(ctx context.Context, id int64) (TaskInfo, bool, error) {
	query, err := tenant.ScopedModel(ctx, s.db, s.infoModel, "")
	if err != nil {
		return TaskInfo{}, false, err
	}
	query = taskTimeModel(query)
	var info TaskInfo
	if err = query.Where("id", id).Scan(&info); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TaskInfo{}, false, nil
		}
		return TaskInfo{}, false, gerror.Wrap(err, "读取任务失败")
	}
	return info, info.ID > 0, nil
}

// FindInternal 跨租户读取任务，仅供模块后台流程使用。
func (s *Store) FindInternal(ctx context.Context, id int64) (TaskInfo, bool, error) {
	return s.Find(tenant.WithoutTenant(ctx), id)
}

// ListEnabled 跨租户读取全部启用任务。
func (s *Store) ListEnabled(ctx context.Context) ([]TaskInfo, error) {
	query, err := tenant.ScopedModel(tenant.WithoutTenant(ctx), s.db, s.infoModel, "")
	if err != nil {
		return nil, err
	}
	query = taskTimeModel(query)
	items := []TaskInfo{}
	if err = query.Where("status", 1).OrderAsc("id").Scan(&items); err != nil {
		return nil, gerror.Wrap(err, "读取启用任务失败")
	}
	return items, nil
}

// Insert 在当前租户作用域新增任务。
func (s *Store) Insert(ctx context.Context, data TaskInfoDO) (int64, error) {
	query, err := tenant.ScopedModel(ctx, s.db, s.infoModel, "")
	if err != nil {
		return 0, err
	}
	query = taskTimeModel(query)
	id, err := query.OmitNilData().Data(data).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "新增任务失败")
	}
	return id, nil
}

// Update 在当前租户作用域更新任务。
func (s *Store) Update(ctx context.Context, id int64, data TaskInfoDO) error {
	query, err := tenant.ScopedModel(ctx, s.db, s.infoModel, "")
	if err != nil {
		return err
	}
	query = taskTimeModel(query)
	result, err := query.Where("id", id).OmitNilData().Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "更新任务失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "读取任务更新结果失败")
	}
	if affected == 0 {
		return gerror.New("任务不存在")
	}
	return nil
}

// UpdateRuntimeForJob 仅更新当前调度世代的运行字段。
func (s *Store) UpdateRuntimeForJob(ctx context.Context, id int64, jobID string, data TaskInfoDO) (bool, error) {
	query, err := tenant.ScopedModel(tenant.WithoutTenant(ctx), s.db, s.infoModel, "")
	if err != nil {
		return false, err
	}
	query = taskTimeModel(query)
	result, err := query.Where("id", id).Where("jobId", jobID).OmitNilData().Data(data).Update()
	if err != nil {
		return false, gerror.Wrap(err, "更新任务运行状态失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, gerror.Wrap(err, "读取任务运行状态更新结果失败")
	}
	if affected > 0 {
		return true, nil
	}
	count, err := query.Where("id", id).Where("jobId", jobID).Count()
	if err != nil {
		return false, gerror.Wrap(err, "校验任务调度世代失败")
	}
	return count == 1, nil
}

// Delete 原子删除当前作用域内任务及日志。
func (s *Store) Delete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return gerror.New("任务 ID 不能为空")
	}
	ctx = recycle.WithBypass(ctx)
	return s.db.Transaction(ctx, func(txCtx context.Context, tx gdb.TX) error {
		return s.deleteWithTX(txCtx, tx, nil, ids)
	})
}

// DeleteManaged 使用回收站 Manager 提供的事务删除任务及日志。
func (s *Store) DeleteManaged(ctx context.Context, scope *recycle.DeleteScope, ids []int64) error {
	if scope == nil || scope.TX() == nil {
		return gerror.New("任务删除缺少回收站事务")
	}
	if len(ids) == 0 {
		return gerror.New("任务 ID 不能为空")
	}
	return s.deleteWithTX(ctx, scope.TX(), scope, ids)
}

// deleteWithTX 在指定事务内归档并物理删除任务聚合。
func (s *Store) deleteWithTX(ctx context.Context, tx gdb.TX, scope *recycle.DeleteScope, ids []int64) error {
	infoLockQuery, err := tenant.ScopedModel(ctx, tx, s.infoModel, "")
	if err != nil {
		return err
	}
	infoLockQuery = taskTimeModel(infoLockQuery)
	rows, err := infoLockQuery.Fields("id").WhereIn("id", ids).OrderAsc("id").LockUpdate().All()
	if err != nil {
		return gerror.Wrap(err, "锁定待删除任务失败")
	}
	if len(rows) != len(ids) {
		return gerror.New("任务不存在")
	}
	logLockQuery, err := tenant.ScopedModel(ctx, tx, s.logModel, "")
	if err != nil {
		return err
	}
	logLockQuery = taskTimeModel(logLockQuery)
	if scope != nil && scope.IsArchiving() {
		logs, queryErr := logLockQuery.WhereIn("taskId", ids).OrderAsc("taskId").OrderAsc("id").LockUpdate().All()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "锁定待删除任务日志失败")
		}
		for _, log := range logs {
			taskID := log["taskId"].Int64()
			parentKey, exists := scope.RootKey(taskID)
			if !exists {
				return gerror.Newf("任务 %d 缺少回收站根归档项", taskID)
			}
			if _, addErr := scope.AddRecord(s.logModel, log.Map(), recycle.ItemOptions{
				BranchKey: strconv.FormatInt(taskID, 10), ParentKey: parentKey, RestoreOrder: 10,
			}); addErr != nil {
				return addErr
			}
		}
	}
	logDeleteQuery, err := tenant.ScopedModel(ctx, tx, s.logModel, "")
	if err != nil {
		return err
	}
	logResult, err := taskTimeModel(logDeleteQuery).WhereIn("taskId", ids).Delete()
	if err != nil {
		return gerror.Wrap(err, "删除任务日志失败")
	}
	if err = markTaskDeleted(scope, logResult); err != nil {
		return err
	}
	infoDeleteQuery, err := tenant.ScopedModel(ctx, tx, s.infoModel, "")
	if err != nil {
		return err
	}
	infoResult, err := taskTimeModel(infoDeleteQuery).WhereIn("id", ids).Delete()
	if err != nil {
		return gerror.Wrap(err, "删除任务失败")
	}
	affected, err := infoResult.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "读取任务删除结果失败")
	}
	if affected != int64(len(ids)) {
		return gerror.Newf("任务删除数量异常: 期望 %d，实际 %d", len(ids), affected)
	}
	if scope != nil {
		return scope.MarkDeleted(affected)
	}
	return nil
}

// markTaskDeleted 记录回收站事务内的物理删除行数。
func markTaskDeleted(scope *recycle.DeleteScope, result sql.Result) error {
	if scope == nil {
		return nil
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "读取任务日志删除结果失败")
	}
	return scope.MarkDeleted(affected)
}

// Claim 通过条件更新领取一次任务执行租约。
func (s *Store) Claim(ctx context.Context, info TaskInfo, payload taskQueue.Payload, claimToken string, lockTTL time.Duration) (TaskLease, bool, error) {
	if claimToken == "" {
		return TaskLease{}, false, gerror.New("领取任务执行租约缺少 token")
	}
	now := time.Now()
	expiresAt := now.Add(lockTTL).Truncate(time.Second)
	batchTime := scheduledBatchTime(payload.ScheduledAt)
	query, err := tenant.ScopedModel(tenant.WithoutTenant(ctx), s.db, s.infoModel, "")
	if err != nil {
		return TaskLease{}, false, err
	}
	query = taskTimeModel(query)
	query = query.Where("id", info.ID).
		Where("jobId", info.JobID).
		Where("lockExpireTime IS NULL OR lockExpireTime<=?", now)
	data := TaskInfoDO{LockExpireTime: expiresAt, LockOwner: claimToken}
	if !payload.Manual && payload.Attempt > 0 {
		query = query.Where("status", info.Status).
			Where("lastExecuteTime", batchTime)
	} else if !payload.Manual {
		state := repeatState(info)
		state.Count++
		encoded, encodeErr := json.Marshal(state)
		if encodeErr != nil {
			return TaskLease{}, false, gerror.Wrap(encodeErr, "编码任务重复状态失败")
		}
		query = query.Where("status", 1).
			Where("lastExecuteTime IS NULL OR lastExecuteTime<?", batchTime)
		data.LastExecuteTime = batchTime
		data.RepeatConf = string(encoded)
		if info.Limit != nil && state.Count >= *info.Limit {
			data.Status = 0
			data.NextRunTime = gdb.Raw("NULL")
		}
	}
	result, err := query.Data(data).Update()
	if err != nil {
		return TaskLease{}, false, gerror.Wrap(err, "领取任务执行租约失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return TaskLease{}, false, gerror.Wrap(err, "读取任务租约结果失败")
	}
	if affected != 1 {
		return TaskLease{}, false, nil
	}
	return TaskLease{Token: claimToken, ExpiresAt: expiresAt}, true, nil
}

// Renew 延长当前所有者的任务租约。
func (s *Store) Renew(ctx context.Context, info TaskInfo, claimToken string, lockTTL time.Duration) (time.Time, error) {
	if claimToken == "" {
		return time.Time{}, gerror.New("续期任务执行租约缺少 token")
	}
	now := time.Now()
	expiresAt := now.Add(lockTTL).Truncate(time.Second)
	query, err := tenant.ScopedModel(tenant.WithoutTenant(ctx), s.db, s.infoModel, "")
	if err != nil {
		return time.Time{}, err
	}
	query = taskTimeModel(query)
	_, err = query.Where("id", info.ID).Where("jobId", info.JobID).Where("lockOwner", claimToken).
		Where("lockExpireTime>?", now).Data(TaskInfoDO{LockExpireTime: expiresAt}).Update()
	if err != nil {
		return time.Time{}, gerror.Wrap(err, "续期任务执行租约失败")
	}
	var current TaskInfo
	err = query.Where("id", info.ID).Where("jobId", info.JobID).Where("lockOwner", claimToken).
		Where("lockExpireTime>?", now).Scan(&current)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, ErrTaskLeaseLost
		}
		return time.Time{}, gerror.Wrap(err, "确认任务续租结果失败")
	}
	if current.ID == 0 || current.LockExpireTime == nil || !current.LockExpireTime.Time.After(now) {
		return time.Time{}, ErrTaskLeaseLost
	}
	return current.LockExpireTime.Time, nil
}

// Release 释放当前所有者的任务租约。
func (s *Store) Release(ctx context.Context, info TaskInfo, claimToken string) error {
	if claimToken == "" {
		return gerror.New("释放任务执行租约缺少 token")
	}
	query, err := tenant.ScopedModel(tenant.WithoutTenant(ctx), s.db, s.infoModel, "")
	if err != nil {
		return err
	}
	query = taskTimeModel(query)
	_, err = query.Where("id", info.ID).Where("jobId", info.JobID).Where("lockOwner", claimToken).
		Where("lockExpireTime>?", time.Now()).
		Data(TaskInfoDO{LockExpireTime: gdb.Raw("NULL"), LockOwner: gdb.Raw("NULL")}).Update()
	return gerror.Wrap(err, "释放任务执行租约失败")
}

// SaveRepeatState 保存当前世代触发次数。
func (s *Store) SaveRepeatState(ctx context.Context, info TaskInfo, state RepeatState, shouldStop bool) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return gerror.Wrap(err, "编码任务重复状态失败")
	}
	data := TaskInfoDO{RepeatConf: string(encoded)}
	if shouldStop {
		data.Status = 0
		data.NextRunTime = gdb.Raw("NULL")
	}
	_, err = s.UpdateRuntimeForJob(ctx, info.ID, info.JobID, data)
	return err
}

// WriteLog 在任务所属租户下写入执行日志。
func (s *Store) WriteLog(ctx context.Context, info TaskInfo, status int, detail string) error {
	logContext, err := taskTenantContext(ctx, info.TenantID)
	if err != nil {
		return err
	}
	query, err := tenant.ScopedModel(logContext, s.db, s.logModel, "")
	if err != nil {
		return err
	}
	query = taskTimeModel(query)
	_, err = query.Data(taskLogDO{TaskID: info.ID, Status: status, Detail: detail}).Insert()
	return gerror.Wrap(err, "写入任务日志失败")
}

// CleanupLogs 跨租户删除超过保留期的日志。
func (s *Store) CleanupLogs(ctx context.Context, before time.Time) error {
	const batchSize = 1000

	maintenanceCtx := cleanupLogsContext(ctx)
	for {
		if err := maintenanceCtx.Err(); err != nil {
			return gerror.Wrap(err, "清理任务日志已取消")
		}
		query, err := tenant.ScopedModel(maintenanceCtx, s.db, s.logModel, "")
		if err != nil {
			return err
		}
		result, err := taskTimeModel(query).WhereLT("createTime", before).Limit(batchSize).Delete()
		if err != nil {
			return gerror.Wrap(err, "清理任务日志失败")
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return gerror.Wrap(err, "读取任务日志清理结果失败")
		}
		if affected < batchSize {
			return nil
		}
	}
}

// cleanupLogsContext 创建跨租户且绕过回收归档的日志维护上下文。
func cleanupLogsContext(ctx context.Context) context.Context {
	return recycle.WithBypass(tenant.WithoutTenant(ctx))
}

// LogPage 返回当前租户作用域内的任务日志分页。
func (s *Store) LogPage(ctx context.Context, taskID int64, status *int, page int, size int) (map[string]interface{}, error) {
	query, err := tenant.ScopedModel(ctx, s.db, s.logModel, "a")
	if err != nil {
		return nil, err
	}
	query = taskTimeModel(query)
	query = query.LeftJoin(s.infoModel.TableName+" b", "a.taskId=b.id")
	if taskID > 0 {
		query = query.Where("a.taskId", taskID)
	}
	if status != nil {
		query = query.Where("a.status", *status)
	}
	total, err := query.Clone().Fields("a.id").Count()
	if err != nil {
		return nil, gerror.Wrap(err, "查询任务日志总数失败")
	}
	items := []taskLogRecord{}
	query = query.Fields("a.id,a.createTime,a.updateTime,a.tenantId,a.taskId,a.status,a.detail,b.name AS task_name")
	if err = query.Page(page, size).OrderDesc("a.id").Scan(&items); err != nil {
		return nil, gerror.Wrap(err, "查询任务日志失败")
	}
	list := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		encoded, marshalErr := json.Marshal(item)
		if marshalErr != nil {
			return nil, gerror.Wrap(marshalErr, "编码任务日志失败")
		}
		mapped := map[string]interface{}{}
		if unmarshalErr := json.Unmarshal(encoded, &mapped); unmarshalErr != nil {
			return nil, gerror.Wrap(unmarshalErr, "转换任务日志失败")
		}
		list = append(list, mapped)
	}
	return map[string]interface{}{
		"list": list,
		"pagination": map[string]interface{}{
			"page": page, "size": size, "total": total,
		},
	}, nil
}

func taskTenantContext(ctx context.Context, tenantID *int64) (context.Context, error) {
	if tenantID == nil {
		return tenant.WithoutTenant(ctx), nil
	}
	return tenant.ForTenant(ctx, *tenantID)
}
