package scheduler

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lyallcooper/kuron/internal/db"
	"github.com/lyallcooper/kuron/internal/fclones"
	"github.com/lyallcooper/kuron/internal/services"
)

// mockExecutor implements fclones.ExecutorInterface for testing
type mockExecutor struct {
	mu          sync.Mutex
	groupOutput *fclones.GroupOutput
	groupErr    error
}

func (m *mockExecutor) CheckInstalled(ctx context.Context) error {
	return nil
}

func (m *mockExecutor) Version(ctx context.Context) (string, error) {
	return "test", nil
}

func (m *mockExecutor) Group(ctx context.Context, opts fclones.ScanOptions, progressChan chan<- fclones.Progress) (*fclones.GroupOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.groupOutput, m.groupErr
}

func (m *mockExecutor) GroupToInput(groups []fclones.Group) string {
	return ""
}

func (m *mockExecutor) Link(ctx context.Context, input string, opts fclones.LinkOptions) (string, error) {
	return "", nil
}

func (m *mockExecutor) Dedupe(ctx context.Context, input string, opts fclones.DedupeOptions) (string, error) {
	return "", nil
}

func (m *mockExecutor) Remove(ctx context.Context, input string, opts fclones.RemoveOptions) (string, error) {
	return "", nil
}

func testDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestNew(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{
		groupOutput: &fclones.GroupOutput{Header: fclones.Header{Stats: fclones.Stats{}}},
	}
	scanner := services.NewScanner(database, executor, 5*time.Minute)

	s := New(database, scanner)

	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.db != database {
		t.Error("scheduler.db not set correctly")
	}
	if s.scanner != scanner {
		t.Error("scheduler.scanner not set correctly")
	}
	if s.running {
		t.Error("scheduler should not be running initially")
	}
}

func TestStartStop(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{
		groupOutput: &fclones.GroupOutput{Header: fclones.Header{Stats: fclones.Stats{}}},
	}
	scanner := services.NewScanner(database, executor, 5*time.Minute)
	s := New(database, scanner)

	// Start scheduler
	s.Start()

	s.mu.RLock()
	running := s.running
	s.mu.RUnlock()

	if !running {
		t.Error("scheduler should be running after Start")
	}

	// Double start should be idempotent
	s.Start()

	// Stop scheduler
	s.Stop()

	s.mu.RLock()
	running = s.running
	s.mu.RUnlock()

	if running {
		t.Error("scheduler should not be running after Stop")
	}

	// Double stop should be safe
	s.Stop()
}

func TestUpdateNextRun(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{
		groupOutput: &fclones.GroupOutput{Header: fclones.Header{Stats: fclones.Stats{}}},
	}
	scanner := services.NewScanner(database, executor, 5*time.Minute)
	s := New(database, scanner)

	// Create a job
	job := &db.ScheduledJob{
		Name:           "Test Job",
		Paths:          []string{"/tmp"},
		Enabled:        true,
		CronExpression: "0 * * * *", // Every hour
		Action:         "scan",
	}
	job, err := database.CreateScheduledJob(job)
	if err != nil {
		t.Fatalf("CreateScheduledJob failed: %v", err)
	}

	// Update next run
	err = s.UpdateNextRun(job)
	if err != nil {
		t.Fatalf("UpdateNextRun failed: %v", err)
	}

	if job.NextRunAt == nil {
		t.Fatal("NextRunAt should be set")
	}

	// Should be within the next hour
	now := time.Now()
	if job.NextRunAt.Before(now) {
		t.Error("NextRunAt should be in the future")
	}
	if job.NextRunAt.After(now.Add(time.Hour)) {
		t.Error("NextRunAt should be within the next hour")
	}
}

func TestUpdateNextRunInvalidCron(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{
		groupOutput: &fclones.GroupOutput{Header: fclones.Header{Stats: fclones.Stats{}}},
	}
	scanner := services.NewScanner(database, executor, 5*time.Minute)
	s := New(database, scanner)

	job := &db.ScheduledJob{
		Name:           "Invalid Job",
		Paths:          []string{"/tmp"},
		Enabled:        true,
		CronExpression: "invalid cron",
		Action:         "scan",
	}
	job, _ = database.CreateScheduledJob(job)

	err := s.UpdateNextRun(job)
	if err == nil {
		t.Error("UpdateNextRun should fail with invalid cron expression")
	}
}

func TestCronExpressionParsing(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{
		groupOutput: &fclones.GroupOutput{Header: fclones.Header{Stats: fclones.Stats{}}},
	}
	scanner := services.NewScanner(database, executor, 5*time.Minute)
	s := New(database, scanner)

	tests := []struct {
		name    string
		cron    string
		wantErr bool
	}{
		{"every minute", "* * * * *", false},
		{"every hour", "0 * * * *", false},
		{"daily at midnight", "0 0 * * *", false},
		{"weekly on sunday", "0 0 * * 0", false},
		{"monthly first day", "0 0 1 * *", false},
		{"invalid", "invalid", true},
		{"too few fields", "* * *", true},
		{"too many fields", "* * * * * *", true}, // 6 fields (with seconds) not supported by our parser
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &db.ScheduledJob{
				Name:           tt.name,
				Paths:          []string{"/tmp"},
				Enabled:        true,
				CronExpression: tt.cron,
				Action:         "scan",
			}
			job, _ = database.CreateScheduledJob(job)

			err := s.UpdateNextRun(job)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateNextRun() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckJobsFiltersCorrectly(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{
		groupOutput: &fclones.GroupOutput{Header: fclones.Header{Stats: fclones.Stats{}}},
	}
	scanner := services.NewScanner(database, executor, 5*time.Minute)
	_ = New(database, scanner) // Scheduler not directly used, just testing DB filtering

	// Create enabled job with past next run time (should trigger)
	pastTime := time.Now().Add(-time.Hour)
	enabledJob := &db.ScheduledJob{
		Name:           "Enabled Job",
		Paths:          []string{"/tmp"},
		Enabled:        true,
		CronExpression: "0 * * * *",
		Action:         "scan",
		NextRunAt:      &pastTime,
	}
	enabledJob, _ = database.CreateScheduledJob(enabledJob)

	// Create disabled job (should not trigger)
	disabledJob := &db.ScheduledJob{
		Name:           "Disabled Job",
		Paths:          []string{"/tmp"},
		Enabled:        false,
		CronExpression: "0 * * * *",
		Action:         "scan",
		NextRunAt:      &pastTime,
	}
	disabledJob, _ = database.CreateScheduledJob(disabledJob)

	// Create enabled job with future next run time (should not trigger)
	futureTime := time.Now().Add(time.Hour)
	futureJob := &db.ScheduledJob{
		Name:           "Future Job",
		Paths:          []string{"/tmp"},
		Enabled:        true,
		CronExpression: "0 * * * *",
		Action:         "scan",
		NextRunAt:      &futureTime,
	}
	futureJob, _ = database.CreateScheduledJob(futureJob)

	// Get enabled jobs (what checkJobs uses)
	jobs, err := database.GetEnabledJobs()
	if err != nil {
		t.Fatalf("GetEnabledJobs failed: %v", err)
	}

	// Should only get enabled jobs
	if len(jobs) != 2 {
		t.Errorf("expected 2 enabled jobs, got %d", len(jobs))
	}

	// Verify disabled job is not included
	for _, j := range jobs {
		if j.ID == disabledJob.ID {
			t.Error("disabled job should not be in enabled jobs list")
		}
	}
}

func TestGracefulShutdown(t *testing.T) {
	database := testDB(t)

	// Create executor that blocks
	blockingExecutor := &blockingExecutor{
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}

	scanner := services.NewScanner(database, blockingExecutor, 5*time.Minute)
	s := New(database, scanner)

	// Create job that will trigger immediately
	pastTime := time.Now().Add(-time.Hour)
	job := &db.ScheduledJob{
		Name:           "Blocking Job",
		Paths:          []string{"/tmp"},
		Enabled:        true,
		CronExpression: "0 * * * *",
		Action:         "scan",
		NextRunAt:      &pastTime,
	}
	database.CreateScheduledJob(job)

	// Start scheduler
	s.Start()

	// Wait for job to start
	select {
	case <-blockingExecutor.started:
		// Job started
	case <-time.After(2 * time.Second):
		t.Fatal("job did not start")
	}

	// Stop should cancel context and wait
	stopDone := make(chan struct{})
	go func() {
		s.Stop()
		close(stopDone)
	}()

	// Signal job to finish
	close(blockingExecutor.done)

	// Stop should complete
	select {
	case <-stopDone:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not complete in time")
	}
}

// blockingExecutor blocks until signaled
type blockingExecutor struct {
	started chan struct{}
	done    chan struct{}
	once    sync.Once
}

func (m *blockingExecutor) CheckInstalled(ctx context.Context) error {
	return nil
}

func (m *blockingExecutor) Version(ctx context.Context) (string, error) {
	return "test", nil
}

func (m *blockingExecutor) Group(ctx context.Context, opts fclones.ScanOptions, progressChan chan<- fclones.Progress) (*fclones.GroupOutput, error) {
	m.once.Do(func() { close(m.started) })
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.done:
		return &fclones.GroupOutput{
			Header: fclones.Header{Stats: fclones.Stats{}},
		}, nil
	}
}

func (m *blockingExecutor) GroupToInput(groups []fclones.Group) string {
	return ""
}

func (m *blockingExecutor) Link(ctx context.Context, input string, opts fclones.LinkOptions) (string, error) {
	return "", nil
}

func (m *blockingExecutor) Dedupe(ctx context.Context, input string, opts fclones.DedupeOptions) (string, error) {
	return "", nil
}

func (m *blockingExecutor) Remove(ctx context.Context, input string, opts fclones.RemoveOptions) (string, error) {
	return "", nil
}
