package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/task"
	"github.com/toothdy/cool-admin-go-next/modules/task/dto"
)

const (
	minTaskEveryMilliseconds int64 = 1000
	maxTaskEveryMilliseconds int64 = 100000000000
)

// InfoEngine 描述任务管理服务需要的 Engine 能力。
type InfoEngine interface {
	Healthy(ctx context.Context) error
	SyncTask(ctx context.Context, taskID int64) error
	RemoveTask(ctx context.Context, taskID int64) error
	Once(ctx context.Context, info TaskInfo) error
}

// InfoService 提供 Node 兼容的任务管理业务。
type InfoService struct {
	store    *Store
	registry *task.Registry
	engine   InfoEngine
	location *time.Location
	recycle  recycle.DeleteManager
}

// InfoLogRequest 描述任务日志分页请求(已移至 task/dto/info.go)

type infoDraft struct {
	Name      string
	Cron      string
	Limit     *int
	Every     *int64
	Remark    string
	Status    int
	StartDate *time.Time
	EndDate   *time.Time
	Data      string
	Service   string
	Type      int
	TaskType  int
}

// BuildInfoService 创建 TaskInfo 应用服务。
func BuildInfoService(
	store *Store,
	registry *task.Registry,
	engine InfoEngine,
	location *time.Location,
	managers ...recycle.DeleteManager,
) (*InfoService, error) {
	if store == nil || registry == nil || engine == nil || location == nil {
		return nil, gerror.New("InfoService 依赖不完整")
	}
	if len(managers) > 1 {
		return nil, gerror.New("InfoService 只能注入一个回收站 Manager")
	}
	service := &InfoService{store: store, registry: registry, engine: engine, location: location}
	if len(managers) == 1 {
		service.recycle = managers[0]
	}
	return service, nil
}

// Healthy 检查任务调度后端能否接收写操作。
func (s *InfoService) Healthy(ctx context.Context) error {
	return s.engine.Healthy(ctx)
}

// Add 新增任务并同步运行计划。
func (s *InfoService) Add(ctx context.Context, request crud.AddRequest) (interface{}, error) {
	if err := s.requireHealthy(ctx); err != nil {
		return nil, err
	}
	draft, err := s.parseDraft(request.Data, nil)
	if err != nil {
		return nil, err
	}
	jobID := uuid.NewString()
	id, err := s.store.Insert(ctx, draftDO(draft, jobID, true))
	if err != nil {
		return nil, err
	}
	if draft.Status == 1 {
		s.syncAfterCommit(ctx, id)
	}
	return map[string]interface{}{"id": id}, nil
}

// Update 更新任务并按需替换调度世代。
func (s *InfoService) Update(ctx context.Context, request crud.UpdateRequest) (interface{}, error) {
	if err := s.requireHealthy(ctx); err != nil {
		return nil, err
	}
	id, err := requiredTaskID(request.Data["id"])
	if err != nil {
		return nil, err
	}
	current, exists, err := s.store.Find(ctx, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, exception.Comm("数据不存在")
	}
	draft, err := s.parseDraft(request.Data, &current)
	if err != nil {
		return nil, err
	}
	jobID := current.JobID
	shouldReset := infoScheduleChanged(current, draft)
	if shouldReset {
		jobID = uuid.NewString()
	}
	if err = s.store.Update(ctx, id, draftDO(draft, jobID, shouldReset)); err != nil {
		return nil, err
	}
	if draft.Status == 1 {
		s.syncAfterCommit(ctx, id)
	} else {
		_ = s.engine.RemoveTask(ctx, id)
	}
	return nil, nil
}

// Delete 原子删除任务及其日志。
func (s *InfoService) Delete(ctx context.Context, request crud.DeleteRequest) (interface{}, error) {
	ids, requestIDs, err := taskDeleteIDs(request.IDs)
	if err != nil {
		return nil, err
	}
	if s.recycle == nil {
		err = s.store.Delete(ctx, ids)
	} else {
		err = s.recycle.RunDelete(ctx, recycle.DeleteRequest{
			Resource: s.store.infoModel.ResourceKey(),
			Entity:   s.store.infoModel.Name,
			Model:    s.store.infoModel,
			IDs:      requestIDs,
			Params:   request,
		}, func(txCtx context.Context, scope *recycle.DeleteScope) error {
			if deleteErr := s.store.DeleteManaged(txCtx, scope, ids); deleteErr != nil {
				return deleteErr
			}
			return scope.AfterCommit(removeTasksAfterCommit(s.engine, ids))
		})
	}
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			return nil, exception.Comm("数据不存在")
		}
		return nil, err
	}
	if s.recycle == nil {
		_ = removeTasksAfterCommit(s.engine, ids)(ctx)
	}
	return nil, nil
}

// taskDeleteIDs 校验并去重任务删除 ID。
func taskDeleteIDs(values []interface{}) ([]int64, []interface{}, error) {
	ids := make([]int64, 0, len(values))
	requestIDs := make([]interface{}, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		id, err := requiredTaskID(value)
		if err != nil {
			return nil, nil, err
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		requestIDs = append(requestIDs, id)
	}
	if len(ids) == 0 {
		return nil, nil, exception.Validate("任务 ID 不能为空")
	}
	return ids, requestIDs, nil
}

// removeTasksAfterCommit 创建数据库提交后的调度移除动作。
func removeTasksAfterCommit(engine InfoEngine, ids []int64) func(context.Context) error {
	return func(ctx context.Context) error {
		for _, id := range ids {
			if err := engine.RemoveTask(ctx, id); err != nil {
				g.Log().Errorf(ctx, "删除任务 %d 后移除运行计划失败: %v", id, err)
			}
		}
		return nil
	}
}

// Info 返回 Node 兼容的任务详情。
func (s *InfoService) Info(ctx context.Context, request crud.InfoRequest) (interface{}, error) {
	id, err := requiredTaskID(request.ID)
	if err != nil {
		return nil, err
	}
	info, exists, err := s.store.Find(ctx, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	return taskInfoMap(info), nil
}

// Start 启用任务并创建新调度世代。
func (s *InfoService) Start(ctx context.Context, id int64, taskType *int) error {
	if err := s.requireHealthy(ctx); err != nil {
		return err
	}
	info, exists, err := s.store.Find(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return exception.Comm("数据不存在")
	}
	data := taskInfoMap(info)
	data["status"] = 1
	if taskType != nil {
		data["type"] = *taskType
	}
	draft, err := s.parseDraft(data, &info)
	if err != nil {
		return err
	}
	jobID := uuid.NewString()
	if err = s.store.Update(ctx, id, draftDO(draft, jobID, true)); err != nil {
		return err
	}
	s.syncAfterCommit(ctx, id)
	return nil
}

// Stop 停止任务并清空下次执行时间。
func (s *InfoService) Stop(ctx context.Context, id int64) error {
	if err := s.requireHealthy(ctx); err != nil {
		return err
	}
	if _, exists, err := s.store.Find(ctx, id); err != nil {
		return err
	} else if !exists {
		return exception.Comm("数据不存在")
	}
	if err := s.store.Update(ctx, id, TaskInfoDO{Status: 0, NextRunTime: gdb.Raw("NULL")}); err != nil {
		return err
	}
	return s.engine.RemoveTask(ctx, id)
}

// Once 提交一次手动执行且不消耗周期次数。
func (s *InfoService) Once(ctx context.Context, id int64) error {
	if err := s.requireHealthy(ctx); err != nil {
		return err
	}
	info, exists, err := s.store.Find(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return exception.Comm("数据不存在")
	}
	if err = s.validateHandler(info.Service); err != nil {
		return err
	}
	return s.engine.Once(ctx, info)
}

// Log 返回当前租户下的任务日志分页。
func (s *InfoService) Log(ctx context.Context, request dto.InfoLogRequest) (map[string]interface{}, error) {
	if request.Page <= 0 {
		request.Page = 1
	}
	if request.Size <= 0 {
		request.Size = 20
	}
	if request.Size > 200 {
		request.Size = 200
	}
	return s.store.LogPage(ctx, request.ID, request.Status, request.Page, request.Size)
}

func (s *InfoService) parseDraft(data map[string]interface{}, current *TaskInfo) (infoDraft, error) {
	draft := infoDraft{Status: 1}
	if current != nil {
		draft = infoDraftFromTask(*current)
	}
	applyInfoTrimmedString(data, "name", &draft.Name)
	applyInfoTrimmedString(data, "cron", &draft.Cron)
	applyInfoTrimmedString(data, "remark", &draft.Remark)
	applyInfoString(data, "data", &draft.Data)
	applyInfoTrimmedString(data, "service", &draft.Service)
	if err := applyInfoInt(data, "status", &draft.Status); err != nil {
		return infoDraft{}, exception.Validate("status 必须是整数")
	}
	if err := applyInfoInt(data, "type", &draft.Type); err != nil {
		return infoDraft{}, exception.Validate("type 必须是整数")
	}
	if err := applyInfoInt(data, "taskType", &draft.TaskType); err != nil {
		return infoDraft{}, exception.Validate("taskType 必须是整数")
	}
	if value, exists := firstInfoPresent(data, "repeatCount", "limit"); exists {
		limit, err := optionalInfoInt(value)
		if err != nil {
			return infoDraft{}, exception.Validate("repeatCount 必须是正整数")
		}
		draft.Limit = limit
	}
	if value, exists := data["every"]; exists {
		every, err := optionalInfoInt64(value)
		if err != nil {
			return infoDraft{}, exception.Validate("every 必须是正整数")
		}
		draft.Every = every
	}
	if value, exists := data["startDate"]; exists {
		parsed, err := optionalInfoTime(value, s.location)
		if err != nil {
			return infoDraft{}, exception.Validate("startDate 格式错误")
		}
		draft.StartDate = parsed
	}
	if value, exists := data["endDate"]; exists {
		parsed, err := optionalInfoTime(value, s.location)
		if err != nil {
			return infoDraft{}, exception.Validate("endDate 格式错误")
		}
		draft.EndDate = parsed
	}
	if draft.TaskType == 0 {
		draft.Every = nil
	} else {
		draft.Cron = ""
	}
	if err := s.validateDraft(draft); err != nil {
		return infoDraft{}, err
	}
	return draft, nil
}

func (s *InfoService) validateDraft(draft infoDraft) error {
	if strings.TrimSpace(draft.Name) == "" || len(draft.Name) > 255 {
		return exception.Validate("任务名称不能为空且不能超过 255 字节")
	}
	if draft.Status != 0 && draft.Status != 1 || draft.Type != 0 && draft.Type != 1 || draft.TaskType != 0 && draft.TaskType != 1 {
		return exception.Validate("任务状态或类型无效")
	}
	if draft.Limit != nil && *draft.Limit <= 0 {
		return exception.Validate("repeatCount 必须大于 0")
	}
	if draft.StartDate != nil && draft.EndDate != nil && draft.StartDate.After(*draft.EndDate) {
		return exception.Validate("开始时间不能晚于结束时间")
	}
	if draft.TaskType == 0 {
		if strings.TrimSpace(draft.Cron) == "" {
			return exception.Validate("Cron 任务必须填写 cron")
		}
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		if _, err := parser.Parse(draft.Cron); err != nil {
			return exception.Validate("cron 表达式无效")
		}
	} else if draft.Every == nil {
		return exception.Validate("间隔任务必须填写 every")
	} else if _, err := taskEveryDuration(*draft.Every); err != nil {
		return exception.Validate(err.Error())
	}
	return s.validateHandler(draft.Service)
}

// taskEveryDuration 校验产品边界并安全转换毫秒间隔。
func taskEveryDuration(every int64) (time.Duration, error) {
	if every < minTaskEveryMilliseconds || every > maxTaskEveryMilliseconds {
		return 0, gerror.Newf("间隔任务 every 必须在 %d 到 %d 毫秒之间", minTaskEveryMilliseconds, maxTaskEveryMilliseconds)
	}
	if every > int64(math.MaxInt64)/int64(time.Millisecond) {
		return 0, gerror.New("间隔任务 every 超出时长转换范围")
	}
	return time.Duration(every) * time.Millisecond, nil
}

func (s *InfoService) validateHandler(value string) error {
	expression, err := task.ParseExpression(value)
	if err != nil {
		return exception.Validate(err.Error())
	}
	if _, isFound := s.registry.Find(expression.Key); !isFound {
		return exception.Validate("任务处理器未注册: " + expression.Key)
	}
	return nil
}

func (s *InfoService) requireHealthy(ctx context.Context) error {
	if err := s.engine.Healthy(ctx); err != nil {
		g.Log().Warningf(ctx, "Task Scheduler 不可用: %v", err)
		return exception.Comm("任务调度服务暂不可用")
	}
	return nil
}

func (s *InfoService) syncAfterCommit(ctx context.Context, id int64) {
	if err := s.engine.SyncTask(ctx, id); err != nil {
		g.Log().Errorf(ctx, "提交后同步任务 %d 失败，将由后台对账重试: %v", id, err)
	}
}

func draftDO(draft infoDraft, jobID string, resetState bool) TaskInfoDO {
	data := TaskInfoDO{
		JobID: jobID, Name: draft.Name, Remark: draft.Remark, Status: draft.Status,
		Data: draft.Data, Service: draft.Service, Type: draft.Type, TaskType: draft.TaskType,
		Cron: nullableInfoString(draft.Cron), Limit: nullableInfoInt(draft.Limit), Every: nullableInfoInt64(draft.Every),
		StartDate: nullableInfoTime(draft.StartDate), EndDate: nullableInfoTime(draft.EndDate),
	}
	if draft.Status == 0 {
		data.NextRunTime = gdb.Raw("NULL")
	}
	if resetState {
		state, _ := json.Marshal(RepeatState{Version: 1, Generation: jobID})
		data.RepeatConf = string(state)
		data.LastExecuteTime = gdb.Raw("NULL")
		data.LockExpireTime = gdb.Raw("NULL")
		data.LockOwner = gdb.Raw("NULL")
	}
	return data
}

func infoDraftFromTask(info TaskInfo) infoDraft {
	return infoDraft{
		Name: info.Name, Cron: info.Cron, Limit: info.Limit, Every: info.Every, Remark: info.Remark,
		Status: info.Status, StartDate: infoGTimePointer(info.StartDate), EndDate: infoGTimePointer(info.EndDate),
		Data: info.Data, Service: info.Service, Type: info.Type, TaskType: info.TaskType,
	}
}

func infoScheduleChanged(current TaskInfo, draft infoDraft) bool {
	return current.Status != draft.Status || current.TaskType != draft.TaskType || current.Cron != draft.Cron ||
		!equalInfoInt(current.Limit, draft.Limit) || !equalInfoInt64(current.Every, draft.Every) ||
		!equalInfoTime(infoGTimePointer(current.StartDate), draft.StartDate) || !equalInfoTime(infoGTimePointer(current.EndDate), draft.EndDate) ||
		current.Service != draft.Service || current.Data != draft.Data
}

func taskInfoMap(info TaskInfo) map[string]interface{} {
	encoded, _ := json.Marshal(info)
	result := map[string]interface{}{}
	_ = json.Unmarshal(encoded, &result)
	if info.Limit == nil {
		result["repeatCount"] = nil
	} else {
		result["repeatCount"] = *info.Limit
	}
	return result
}

func requiredTaskID(value interface{}) (int64, error) {
	parsed, err := taskInt64(value)
	if err != nil || parsed <= 0 {
		return 0, exception.Validate("任务 ID 无效")
	}
	return parsed, nil
}

func taskInt64(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int64:
		return typed, nil
	case *int:
		if typed != nil {
			return int64(*typed), nil
		}
	case *int64:
		if typed != nil {
			return *typed, nil
		}
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) {
			return 0, gerror.New("不是整数")
		}
		return strconv.ParseInt(strconv.FormatFloat(typed, 'f', -1, 64), 10, 64)
	case json.Number:
		return typed.Int64()
	case string:
		return strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
	default:
		return 0, gerror.New("不是整数")
	}
	return 0, gerror.New("不是整数")
}

func optionalInfoInt(value interface{}) (*int, error) {
	if value == nil || strings.TrimSpace(fmt.Sprint(value)) == "" {
		return nil, nil
	}
	parsed, err := taskInt64(value)
	result := int(parsed)
	return &result, err
}

func optionalInfoInt64(value interface{}) (*int64, error) {
	if value == nil || strings.TrimSpace(fmt.Sprint(value)) == "" {
		return nil, nil
	}
	parsed, err := taskInt64(value)
	return &parsed, err
}

func optionalInfoTime(value interface{}, location *time.Location) (*time.Time, error) {
	if value == nil || strings.TrimSpace(fmt.Sprint(value)) == "" {
		return nil, nil
	}
	switch typed := value.(type) {
	case time.Time:
		return &typed, nil
	case *time.Time:
		return typed, nil
	case *gtime.Time:
		if typed == nil {
			return nil, nil
		}
		parsed := typed.Time
		return &parsed, nil
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		parsed, err := time.ParseInLocation(layout, text, location)
		if err == nil {
			return &parsed, nil
		}
	}
	return nil, gerror.New("时间格式错误")
}

func applyInfoString(data map[string]interface{}, key string, target *string) {
	if value, exists := data[key]; exists {
		if value == nil {
			*target = ""
		} else {
			*target = fmt.Sprint(value)
		}
	}
}

func applyInfoTrimmedString(data map[string]interface{}, key string, target *string) {
	applyInfoString(data, key, target)
	*target = strings.TrimSpace(*target)
}

func applyInfoInt(data map[string]interface{}, key string, target *int) error {
	if value, exists := data[key]; exists {
		parsed, err := taskInt64(value)
		if err != nil {
			return err
		}
		*target = int(parsed)
	}
	return nil
}

func firstInfoPresent(data map[string]interface{}, keys ...string) (interface{}, bool) {
	for _, key := range keys {
		if value, exists := data[key]; exists {
			return value, true
		}
	}
	return nil, false
}

func nullableInfoString(value string) interface{} {
	if value == "" {
		return gdb.Raw("NULL")
	}
	return value
}

func nullableInfoInt(value *int) interface{} {
	if value == nil {
		return gdb.Raw("NULL")
	}
	return *value
}

func nullableInfoInt64(value *int64) interface{} {
	if value == nil {
		return gdb.Raw("NULL")
	}
	return *value
}

func nullableInfoTime(value *time.Time) interface{} {
	if value == nil {
		return gdb.Raw("NULL")
	}
	return *value
}

func infoGTimePointer(value *gtime.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.Time
	return &result
}

func equalInfoInt(left *int, right *int) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func equalInfoInt64(left *int64, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func equalInfoTime(left *time.Time, right *time.Time) bool {
	return left == nil && right == nil || left != nil && right != nil && left.Equal(*right)
}
