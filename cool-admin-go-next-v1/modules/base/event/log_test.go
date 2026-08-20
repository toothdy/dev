package event

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	baseModule "github.com/toothdy/cool-admin-go-next/modules/base"
	baseSysService "github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
)

func TestNewLogBuildsRuntimeFromBaseConfig(t *testing.T) {
	config := baseModule.ModuleConfig().Defaults
	config.Middleware.Log.QueueSize = 3
	runtime, err := NewLog(&baseSysService.LogService{}, config)
	if err != nil {
		t.Fatal(err)
	}
	if cap(runtime.queue) != 3 || !runtime.enabled {
		t.Fatalf("unexpected log runtime from Base config: %#v", runtime)
	}
}

type writtenLog struct {
	request baseSysService.LogRecordRequest
	scope   tenant.Scope
}

type recordingLogWriter struct {
	mutex       sync.Mutex
	records     []writtenLog
	cleanupRuns int
}

func (w *recordingLogWriter) Record(ctx context.Context, request baseSysService.LogRecordRequest) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.records = append(w.records, writtenLog{request: request, scope: tenant.Resolve(ctx)})
	return nil
}

func (w *recordingLogWriter) ClearExpired(context.Context) (int64, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.cleanupRuns++
	return 0, nil
}

func (w *recordingLogWriter) Records() []writtenLog {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return append([]writtenLog{}, w.records...)
}

func (w *recordingLogWriter) CleanupRuns() int {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.cleanupRuns
}

type blockingLogWriter struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockingLogWriter) Record(context.Context, baseSysService.LogRecordRequest) error {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return nil
}

func (w *blockingLogWriter) ClearExpired(context.Context) (int64, error) {
	return 0, nil
}

type timeoutLogWriter struct {
	started   chan struct{}
	continued chan string
}

func (w *timeoutLogWriter) Record(ctx context.Context, request baseSysService.LogRecordRequest) error {
	if request.Action == "/timeout" {
		close(w.started)
		<-ctx.Done()
		return ctx.Err()
	}
	w.continued <- request.Action
	return nil
}

func (w *timeoutLogWriter) ClearExpired(context.Context) (int64, error) {
	return 0, nil
}

type panicLogWriter struct {
	continued chan string
}

func (w *panicLogWriter) Record(_ context.Context, request baseSysService.LogRecordRequest) error {
	if request.Action == "/panic" {
		panic("operation log write failed")
	}
	w.continued <- request.Action
	return nil
}

func (w *panicLogWriter) ClearExpired(context.Context) (int64, error) {
	return 0, nil
}

type blockingCleanupStore struct {
	mutex   sync.Mutex
	started chan struct{}
	release chan struct{}
	runs    int
}

func (s *blockingCleanupStore) Record(context.Context, baseSysService.LogRecordRequest) error {
	return nil
}

func (s *blockingCleanupStore) ClearExpired(ctx context.Context) (int64, error) {
	s.mutex.Lock()
	s.runs++
	if s.runs == 1 {
		close(s.started)
	}
	s.mutex.Unlock()
	select {
	case <-s.release:
		return 0, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (s *blockingCleanupStore) Runs() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.runs
}

type panicCleanupStore struct {
	mutex sync.Mutex
	runs  int
}

func (s *panicCleanupStore) Record(context.Context, baseSysService.LogRecordRequest) error {
	return nil
}

func (s *panicCleanupStore) ClearExpired(context.Context) (int64, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.runs++
	if s.runs == 1 {
		panic("operation log cleanup failed")
	}
	return 0, nil
}

func (s *panicCleanupStore) Runs() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.runs
}

func TestLogSubmitDoesNotWaitForWriterAndDropsWhenFull(t *testing.T) {
	writer := &blockingLogWriter{started: make(chan struct{}), release: make(chan struct{})}
	runtime, err := BuildLog(writer, LogOptions{Enabled: true, QueueSize: 1, ShutdownTimeout: time.Second, WriteTimeout: time.Second, CleanupTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !runtime.Submit(context.Background(), baseSysService.LogRecordRequest{Action: "/first"}) {
		t.Fatal("expected first log accepted")
	}
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("writer did not start")
	}
	if !runtime.Submit(context.Background(), baseSysService.LogRecordRequest{Action: "/second"}) {
		t.Fatal("expected second log queued")
	}
	started := time.Now()
	if runtime.Submit(context.Background(), baseSysService.LogRecordRequest{Action: "/third"}) {
		t.Fatal("expected full queue to reject latest log")
	}
	if time.Since(started) > 20*time.Millisecond {
		t.Fatal("full log queue blocked request")
	}
	if runtime.Dropped() != 1 {
		t.Fatalf("expected one dropped log, got %d", runtime.Dropped())
	}
	close(writer.release)
	if err = runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLogStopFlushesQueueAndRebuildsTenantScope(t *testing.T) {
	writer := &recordingLogWriter{}
	runtime, err := BuildLog(writer, LogOptions{Enabled: true, QueueSize: 4, ShutdownTimeout: time.Second, WriteTimeout: time.Second, CleanupTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	tenantContext, err := tenant.ForTenant(context.Background(), 17)
	if err != nil {
		t.Fatal(err)
	}
	userID := int64(5)
	if !runtime.Submit(tenantContext, baseSysService.LogRecordRequest{UserID: &userID, Action: "/tenant"}) {
		t.Fatal("expected tenant log accepted")
	}
	userID = 99
	if !runtime.Submit(context.Background(), baseSysService.LogRecordRequest{Action: "/anonymous"}) {
		t.Fatal("expected anonymous log accepted")
	}
	if err = runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	records := writer.Records()
	if len(records) != 2 {
		t.Fatalf("expected two flushed logs, got %#v", records)
	}
	if records[0].request.UserID == nil || *records[0].request.UserID != 5 {
		t.Fatalf("queued log was mutated after submit: %#v", records[0].request)
	}
	if tenantID, ok := records[0].scope.TenantID(); !ok || tenantID != 17 {
		t.Fatalf("unexpected tenant log scope: %#v", records[0].scope)
	}
	if records[1].scope.Kind() != tenant.KindBypass {
		t.Fatalf("unexpected anonymous log scope: %#v", records[1].scope)
	}
	if runtime.Submit(context.Background(), baseSysService.LogRecordRequest{Action: "/late"}) {
		t.Fatal("stopped runtime accepted a log")
	}
}

func TestLogWriteTimeoutDoesNotStopWorker(t *testing.T) {
	writer := &timeoutLogWriter{started: make(chan struct{}), continued: make(chan string, 1)}
	runtime, err := BuildLog(writer, LogOptions{
		Enabled: true, QueueSize: 2, ShutdownTimeout: time.Second, WriteTimeout: 20 * time.Millisecond,
		CleanupTimeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !runtime.Submit(context.Background(), baseSysService.LogRecordRequest{Action: "/timeout"}) {
		t.Fatal("expected timeout log accepted")
	}
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("timeout writer did not start")
	}
	if !runtime.Submit(context.Background(), baseSysService.LogRecordRequest{Action: "/continued"}) {
		t.Fatal("expected second log accepted")
	}
	select {
	case action := <-writer.continued:
		if action != "/continued" {
			t.Fatalf("unexpected continued action: %s", action)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not continue after write timeout")
	}
	if err = runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLogWritePanicDoesNotStopWorker(t *testing.T) {
	writer := &panicLogWriter{continued: make(chan string, 1)}
	runtime, err := BuildLog(writer, LogOptions{
		Enabled: true, QueueSize: 2, ShutdownTimeout: time.Second, WriteTimeout: time.Second,
		CleanupTimeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !runtime.Submit(context.Background(), baseSysService.LogRecordRequest{Action: "/panic"}) {
		t.Fatal("expected panic log accepted")
	}
	if !runtime.Submit(context.Background(), baseSysService.LogRecordRequest{Action: "/continued"}) {
		t.Fatal("expected second log accepted")
	}
	select {
	case action := <-writer.continued:
		if action != "/continued" {
			t.Fatalf("unexpected continued action: %s", action)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not continue after write panic")
	}
	if err = runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLogCleanupTimeoutEndsRun(t *testing.T) {
	store := &blockingCleanupStore{started: make(chan struct{})}
	runtime, err := BuildLog(store, LogOptions{
		Enabled: false, QueueSize: 1, ShutdownTimeout: time.Second, WriteTimeout: time.Second,
		CleanupTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	runtime.clearExpired(context.Background())
	if time.Since(started) > time.Second {
		t.Fatal("cleanup did not stop after timeout")
	}
	if store.Runs() != 1 || runtime.cleanupRunning.Load() {
		t.Fatalf("unexpected cleanup timeout state: runs=%d running=%t", store.Runs(), runtime.cleanupRunning.Load())
	}
}

func TestLogCleanupSkipsOverlappingRun(t *testing.T) {
	store := &blockingCleanupStore{started: make(chan struct{}), release: make(chan struct{})}
	runtime, err := BuildLog(store, LogOptions{
		Enabled: false, QueueSize: 1, ShutdownTimeout: time.Second, WriteTimeout: time.Second,
		CleanupTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		runtime.clearExpired(context.Background())
		close(done)
	}()
	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not start")
	}
	started := time.Now()
	runtime.clearExpired(context.Background())
	if time.Since(started) > 200*time.Millisecond {
		t.Fatal("overlapping cleanup did not return immediately")
	}
	if store.Runs() != 1 {
		t.Fatalf("overlapping cleanup reached store: runs=%d", store.Runs())
	}
	close(store.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("first cleanup did not finish")
	}
}

func TestLogCleanupPanicDoesNotBlockLaterRun(t *testing.T) {
	store := &panicCleanupStore{}
	runtime, err := BuildLog(store, LogOptions{
		Enabled: false, QueueSize: 1, ShutdownTimeout: time.Second, WriteTimeout: time.Second,
		CleanupTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime.clearExpired(context.Background())
	runtime.clearExpired(context.Background())
	if store.Runs() != 2 || runtime.cleanupRunning.Load() {
		t.Fatalf("cleanup did not recover from panic: runs=%d running=%t", store.Runs(), runtime.cleanupRunning.Load())
	}
}

func TestDisabledLogRuntimeDoesNotRequireWriter(t *testing.T) {
	runtime, err := BuildLog(nil, LogOptions{Enabled: false, QueueSize: 1, ShutdownTimeout: time.Second, WriteTimeout: time.Second, CleanupTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runtime.Submit(context.Background(), baseSysService.LogRecordRequest{}) {
		t.Fatal("disabled runtime accepted a log")
	}
	if err = runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLogRuntimeSchedulesDailyCleanupWithoutRunningAtStartup(t *testing.T) {
	store := &recordingLogWriter{}
	runtime, err := BuildLog(store, LogOptions{Enabled: false, QueueSize: 1, ShutdownTimeout: time.Second, WriteTimeout: time.Second, CleanupTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runtime.scheduler == nil {
		t.Fatal("expected daily cleanup scheduler")
	}
	entries := runtime.scheduler.Entries()
	if len(entries) != 1 {
		t.Fatal("expected one daily cleanup schedule")
	}
	next := entries[0].Next.In(time.Local)
	if next.IsZero() || next.Hour() != 0 || next.Minute() != 0 || next.Second() != 0 {
		t.Fatalf("expected next cleanup at local midnight, got %s", next)
	}
	if store.CleanupRuns() != 0 {
		t.Fatal("cleanup ran immediately at startup")
	}
	if err = runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}
