package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/modules/task"
	"github.com/toothdy/cool-admin-go-next/modules/task/entity"
)

// 单次执行占用任务的租约时长
const executeLease = 5 * time.Minute

// 执行结果
type Outcome struct {
	Executed bool // 是否真正执行
	Removed  bool // 任务已不存在或已停止
}

// 触发来源
type trigger uint8

const (
	triggerScheduled trigger = iota // 定时触发
	triggerManual                   // 手动执行一次
)

type executeRow struct {
	ID             uint64      `orm:"id"`
	Status         int32       `orm:"status"`
	Service        *string     `orm:"service"`
	StartDate      *gtime.Time `orm:"startDate"`
	EndDate        *gtime.Time `orm:"endDate"`
	LockExpireTime *gtime.Time `orm:"lockExpireTime"`
}

type leaseWrite struct {
	LastExecuteTime *gtime.Time `orm:"lastExecuteTime"`
	LockExpireTime  *gtime.Time `orm:"lockExpireTime"`
}

type releaseWrite struct {
	LockExpireTime *gtime.Time `orm:"lockExpireTime"`
}

type logWrite struct {
	TaskID *uint64 `orm:"taskId"`
	Status int32   `orm:"status"`
	Detail *string `orm:"detail"`
}

// 任务执行、结果落库与日志清理
type Executor struct {
	runtime  *db.Runtime
	infoBase *coreservice.Base[entity.Info, uint64]
	logBase  *coreservice.Base[entity.Log, uint64]
	registry *Registry
	keepDays int
}

// 任务执行器
func NewExecutor(
	runtime *db.Runtime,
	infoBase *coreservice.Base[entity.Info, uint64],
	logBase *coreservice.Base[entity.Log, uint64],
	registry *Registry,
	config task.Config,
) (*Executor, error) {
	if runtime == nil || runtime.Runner() == nil || infoBase == nil || infoBase.Descriptor() == nil ||
		logBase == nil || logBase.Descriptor() == nil || registry == nil || config.Log.KeepDays <= 0 {
		return nil, exception.Core("任务执行器依赖无效")
	}

	return &Executor{
		runtime:  runtime,
		infoBase: infoBase,
		logBase:  logBase,
		registry: registry,
		keepDays: config.Log.KeepDays,
	}, nil
}

// 定时触发一次
func (executor *Executor) Execute(ctx context.Context, taskID uint64) (Outcome, error) {
	return executor.execute(ctx, taskID, triggerScheduled)
}

// 手动执行一次，与 Node 一致：不要求任务处于运行状态
func (executor *Executor) ExecuteOnce(ctx context.Context, taskID uint64) error {
	outcome, err := executor.execute(ctx, taskID, triggerManual)
	if err != nil {
		return err
	}
	if outcome.Removed {
		return exception.Comm("任务不存在")
	}

	return nil
}

func (executor *Executor) execute(ctx context.Context, taskID uint64, source trigger) (Outcome, error) {
	if executor == nil || executor.runtime == nil {
		return Outcome{}, exception.Core("任务执行器未初始化")
	}
	current, outcome, err := executor.claim(ctx, taskID, source)
	if err != nil || !outcome.Executed {
		return outcome, err
	}
	status, detail := executor.invoke(ctx, current.Service)

	return outcome, executor.settle(ctx, taskID, status, detail)
}

// 抢占任务执行租约
func (executor *Executor) claim(ctx context.Context, taskID uint64, source trigger) (executeRow, Outcome, error) {
	var (
		current executeRow
		outcome Outcome
	)
	err := executor.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		locked, lockErr := executor.runtime.LockRows(txCtx, executor.infoBase.Descriptor().Table(), []uint64{taskID})
		if lockErr != nil {
			return lockErr
		}
		if len(locked) == 0 {
			outcome.Removed = true
			return nil
		}
		read, modelErr := executor.infoBase.Model(txCtx)
		if modelErr != nil {
			return modelErr
		}
		var row *executeRow
		if modelErr = read.
			Fields("id", "status", "service", "startDate", "endDate", "lockExpireTime").
			Where("id", taskID).
			Scan(&row); modelErr != nil {
			return exception.WrapCore(modelErr, "查询任务失败")
		}
		if row == nil {
			outcome.Removed = true

			return nil
		}
		if source == triggerScheduled && row.Status != entity.StatusRunning {
			outcome.Removed = true

			return nil
		}
		moment := time.Now()
		if !withinExecuteWindow(*row, moment) || leaseHeld(row.LockExpireTime, moment) {
			return nil
		}
		// gdb.Model 默认链式可变，读取时的 Fields 会继续限制写入列，写入必须另取 Model
		write, modelErr := executor.infoBase.Model(txCtx)
		if modelErr != nil {
			return modelErr
		}
		if _, modelErr = write.Where("id", taskID).Data(leaseWrite{
			LastExecuteTime: gtime.New(moment),
			LockExpireTime:  gtime.New(moment.Add(executeLease)),
		}).Update(); modelErr != nil {
			return exception.WrapCore(modelErr, "抢占任务执行租约失败")
		}
		current = *row
		outcome.Executed = true

		return nil
	})

	return current, outcome, err
}

// 记录执行结果、清理超期日志并释放租约
func (executor *Executor) settle(ctx context.Context, taskID uint64, status int32, detail string) error {
	return executor.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		logModel, err := executor.logBase.Model(txCtx)
		if err != nil {
			return err
		}
		if _, err = logModel.Data(logWrite{TaskID: &taskID, Status: status, Detail: &detail}).Insert(); err != nil {
			return exception.WrapCore(err, "写入任务日志失败")
		}
		expired, err := executor.logBase.Model(txCtx)
		if err != nil {
			return err
		}
		if _, err = expired.
			Where("taskId", taskID).
			WhereLT("createTime", time.Now().Local().AddDate(0, 0, -executor.keepDays)).
			Delete(); err != nil {
			return exception.WrapCore(err, "清理任务日志失败")
		}
		infoModel, err := executor.infoBase.Model(txCtx)
		if err != nil {
			return err
		}
		if _, err = infoModel.Where("id", taskID).Data(releaseWrite{}).Update(); err != nil {
			return exception.WrapCore(err, "释放任务执行租约失败")
		}

		return nil
	})
}

// 调用任务目标，panic 与错误统一转成失败日志
func (executor *Executor) invoke(ctx context.Context, expression *string) (status int32, detail string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			status = entity.LogStatusFail
			detail = fmt.Sprint(recovered)
		}
	}()
	if expression == nil || *expression == "" {
		return entity.LogStatusFail, "任务未配置执行目标"
	}
	result, err := executor.registry.Invoke(ctx, *expression)
	if err != nil {
		return entity.LogStatusFail, exception.Resolve(err).Message
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return entity.LogStatusSuccess, ""
	}

	return entity.LogStatusSuccess, string(encoded)
}

func withinExecuteWindow(row executeRow, moment time.Time) bool {
	if row.StartDate != nil && moment.Before(row.StartDate.Time) {
		return false
	}

	return row.EndDate == nil || !moment.After(row.EndDate.Time)
}

func leaseHeld(lockExpireTime *gtime.Time, moment time.Time) bool {
	return lockExpireTime != nil && lockExpireTime.Time.After(moment)
}
