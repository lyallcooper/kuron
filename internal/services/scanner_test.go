package services

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lyallcooper/kuron/internal/db"
	"github.com/lyallcooper/kuron/internal/fclones"
	"github.com/lyallcooper/kuron/internal/types"
)

// mockExecutor implements fclones.ExecutorInterface for testing
type mockExecutor struct {
	mu sync.Mutex

	// Configurable responses
	groupOutput *fclones.GroupOutput
	groupErr    error
	linkOutput  string
	linkErr     error
	dedupeOut   string
	dedupeErr   error
	versionOut  string
	versionErr  error
	checkErr    error

	// Track calls
	groupCalls  int
	linkCalls   int
	dedupeCalls int
}

func (m *mockExecutor) CheckInstalled(ctx context.Context) error {
	return m.checkErr
}

func (m *mockExecutor) Version(ctx context.Context) (string, error) {
	return m.versionOut, m.versionErr
}

func (m *mockExecutor) Group(ctx context.Context, opts fclones.ScanOptions, progressChan chan<- fclones.Progress) (*fclones.GroupOutput, error) {
	m.mu.Lock()
	m.groupCalls++
	m.mu.Unlock()

	// Simulate some progress
	if progressChan != nil {
		progressChan <- fclones.Progress{
			Phase:        "scanning",
			FilesScanned: 100,
			BytesScanned: 1000000,
			PhaseNum:     1,
			PhaseTotal:   6,
			PhaseName:    "Scanning files",
		}
	}

	return m.groupOutput, m.groupErr
}

func (m *mockExecutor) GroupToInput(groups []fclones.Group) string {
	// Simple implementation for testing
	result := ""
	for _, g := range groups {
		result += g.FileHash + "\n"
		for _, f := range g.Files {
			result += "    " + f + "\n"
		}
	}
	return result
}

func (m *mockExecutor) Link(ctx context.Context, input string, opts fclones.LinkOptions) (string, error) {
	m.mu.Lock()
	m.linkCalls++
	m.mu.Unlock()
	return m.linkOutput, m.linkErr
}

func (m *mockExecutor) Dedupe(ctx context.Context, input string, opts fclones.DedupeOptions) (string, error) {
	m.mu.Lock()
	m.dedupeCalls++
	m.mu.Unlock()
	return m.dedupeOut, m.dedupeErr
}

// testDB creates a test database in a temp directory
func testDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestNewScanner(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{}
	timeout := 5 * time.Minute

	scanner := NewScanner(database, executor, timeout)

	if scanner == nil {
		t.Fatal("NewScanner returned nil")
	}
	if scanner.db != database {
		t.Error("scanner.db not set correctly")
	}
	if scanner.executor != executor {
		t.Error("scanner.executor not set correctly")
	}
	if scanner.scanTimeout != timeout {
		t.Errorf("scanner.scanTimeout = %v, want %v", scanner.scanTimeout, timeout)
	}
	if scanner.activeScans == nil {
		t.Error("scanner.activeScans not initialized")
	}
	if scanner.subscribers == nil {
		t.Error("scanner.subscribers not initialized")
	}
}

func TestSubscribeUnsubscribe(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{}
	scanner := NewScanner(database, executor, 5*time.Minute)

	runID := int64(123)

	// Subscribe
	ch := scanner.Subscribe(runID)
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}

	// Verify subscriber was added
	scanner.subMu.RLock()
	subs := scanner.subscribers[runID]
	scanner.subMu.RUnlock()

	if len(subs) != 1 {
		t.Errorf("expected 1 subscriber, got %d", len(subs))
	}

	// Unsubscribe
	scanner.Unsubscribe(runID, ch)

	// Verify subscriber was removed
	scanner.subMu.RLock()
	subs = scanner.subscribers[runID]
	scanner.subMu.RUnlock()

	if len(subs) != 0 {
		t.Errorf("expected 0 subscribers after unsubscribe, got %d", len(subs))
	}
}

func TestMultipleSubscribers(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{}
	scanner := NewScanner(database, executor, 5*time.Minute)

	runID := int64(456)

	// Add multiple subscribers
	ch1 := scanner.Subscribe(runID)
	ch2 := scanner.Subscribe(runID)
	ch3 := scanner.Subscribe(runID)

	scanner.subMu.RLock()
	count := len(scanner.subscribers[runID])
	scanner.subMu.RUnlock()

	if count != 3 {
		t.Errorf("expected 3 subscribers, got %d", count)
	}

	// Unsubscribe middle one
	scanner.Unsubscribe(runID, ch2)

	scanner.subMu.RLock()
	count = len(scanner.subscribers[runID])
	scanner.subMu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 subscribers after unsubscribe, got %d", count)
	}

	// Clean up
	scanner.Unsubscribe(runID, ch1)
	scanner.Unsubscribe(runID, ch3)
}

func TestStartScan(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{
		groupOutput: &fclones.GroupOutput{
			Header: fclones.Header{
				Stats: fclones.Stats{
					GroupCount:         2,
					TotalFileCount:     6,
					TotalFileSize:      100000,
					RedundantFileCount: 4,
					RedundantFileSize:  80000,
				},
			},
			Groups: []fclones.Group{
				{
					FileLen:  10000,
					FileHash: "abc123",
					Files:    []string{"/tmp/file1.txt", "/tmp/file2.txt", "/tmp/file3.txt"},
				},
				{
					FileLen:  20000,
					FileHash: "def456",
					Files:    []string{"/tmp/other1.txt", "/tmp/other2.txt"},
				},
			},
		},
	}
	scanner := NewScanner(database, executor, 5*time.Minute)

	cfg := &ScanConfig{
		Paths:   []string{"/tmp"},
		MinSize: 1024,
	}

	run, err := scanner.StartScan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("StartScan failed: %v", err)
	}

	if run == nil {
		t.Fatal("StartScan returned nil run")
	}
	if run.ID == 0 {
		t.Error("run.ID should be non-zero")
	}
	if run.Status != db.ScanRunStatusRunning {
		t.Errorf("run.Status = %s, want %s", run.Status, db.ScanRunStatusRunning)
	}

	// Wait for scan to complete
	time.Sleep(200 * time.Millisecond)

	// Verify executor was called
	executor.mu.Lock()
	groupCalls := executor.groupCalls
	executor.mu.Unlock()

	if groupCalls != 1 {
		t.Errorf("executor.Group called %d times, want 1", groupCalls)
	}

	// Verify scan completed
	updatedRun, err := database.GetScanRun(run.ID)
	if err != nil {
		t.Fatalf("GetScanRun failed: %v", err)
	}
	if updatedRun.Status != db.ScanRunStatusCompleted {
		t.Errorf("run.Status = %s, want %s", updatedRun.Status, db.ScanRunStatusCompleted)
	}

	// Verify groups were stored
	groups, err := database.ListDuplicateGroups(run.ID, "")
	if err != nil {
		t.Fatalf("ListDuplicateGroups failed: %v", err)
	}
	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}
}

func TestStartScanWithJobID(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{
		groupOutput: &fclones.GroupOutput{
			Header: fclones.Header{Stats: fclones.Stats{}},
		},
	}
	scanner := NewScanner(database, executor, 5*time.Minute)

	// Create a job first
	job := &db.ScheduledJob{
		Name:    "Test Job",
		Paths:   []string{"/tmp"},
		Enabled: true,
	}
	job, err := database.CreateScheduledJob(job)
	if err != nil {
		t.Fatalf("CreateScheduledJob failed: %v", err)
	}

	cfg := &ScanConfig{
		Paths: []string{"/tmp"},
	}

	run, err := scanner.StartScan(context.Background(), cfg, &job.ID)
	if err != nil {
		t.Fatalf("StartScan failed: %v", err)
	}

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	// Verify job ID was recorded
	updatedRun, _ := database.GetScanRun(run.ID)
	if updatedRun.ScheduledJobID == nil {
		t.Error("ScheduledJobID should be set")
	} else if *updatedRun.ScheduledJobID != job.ID {
		t.Errorf("ScheduledJobID = %d, want %d", *updatedRun.ScheduledJobID, job.ID)
	}
}

func TestCancelScan(t *testing.T) {
	database := testDB(t)

	// Create executor that simulates long-running scan
	executor := &mockExecutor{
		groupOutput: nil, // Will be set after we simulate work
	}

	// Override Group to take longer
	scanner := NewScanner(database, executor, 5*time.Minute)

	// Create a custom executor that blocks
	blockingExecutor := &blockingMockExecutor{
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
	scanner.executor = blockingExecutor

	cfg := &ScanConfig{
		Paths: []string{"/tmp"},
	}

	run, err := scanner.StartScan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("StartScan failed: %v", err)
	}

	// Wait for scan to start
	select {
	case <-blockingExecutor.started:
		// Scan started
	case <-time.After(time.Second):
		t.Fatal("scan did not start in time")
	}

	// Cancel the scan
	scanner.CancelScan(run.ID)

	// Signal executor to complete
	close(blockingExecutor.done)

	// Wait a bit for cleanup
	time.Sleep(200 * time.Millisecond)

	// Verify scan was cancelled
	updatedRun, err := database.GetScanRun(run.ID)
	if err != nil {
		t.Fatalf("GetScanRun failed: %v", err)
	}
	if updatedRun.Status != db.ScanRunStatusCancelled {
		t.Errorf("run.Status = %s, want %s", updatedRun.Status, db.ScanRunStatusCancelled)
	}
}

// blockingMockExecutor is a mock that blocks until signaled
type blockingMockExecutor struct {
	started chan struct{}
	done    chan struct{}
}

func (m *blockingMockExecutor) CheckInstalled(ctx context.Context) error {
	return nil
}

func (m *blockingMockExecutor) Version(ctx context.Context) (string, error) {
	return "test", nil
}

func (m *blockingMockExecutor) Group(ctx context.Context, opts fclones.ScanOptions, progressChan chan<- fclones.Progress) (*fclones.GroupOutput, error) {
	close(m.started)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.done:
		return &fclones.GroupOutput{
			Header: fclones.Header{Stats: fclones.Stats{}},
		}, nil
	}
}

func (m *blockingMockExecutor) GroupToInput(groups []fclones.Group) string {
	return ""
}

func (m *blockingMockExecutor) Link(ctx context.Context, input string, opts fclones.LinkOptions) (string, error) {
	return "", nil
}

func (m *blockingMockExecutor) Dedupe(ctx context.Context, input string, opts fclones.DedupeOptions) (string, error) {
	return "", nil
}

func TestExecuteAction(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{
		groupOutput: &fclones.GroupOutput{
			Header: fclones.Header{Stats: fclones.Stats{}},
			Groups: []fclones.Group{
				{FileLen: 1000, FileHash: "hash1", Files: []string{"/a", "/b", "/c"}},
			},
		},
		linkOutput: "Linked 2 files",
	}
	scanner := NewScanner(database, executor, 5*time.Minute)

	// Create a scan run with duplicate groups
	cfg := &ScanConfig{Paths: []string{"/tmp"}}
	run, err := scanner.StartScan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("StartScan failed: %v", err)
	}

	// Wait for scan to complete
	time.Sleep(200 * time.Millisecond)

	// Get the groups
	groups, _ := database.ListDuplicateGroups(run.ID, "")
	if len(groups) == 0 {
		t.Fatal("no groups found")
	}

	groupIDs := []int64{groups[0].ID}

	// Execute hardlink action
	result, err := scanner.ExecuteAction(context.Background(), run.ID, groupIDs, db.ActionTypeHardlink, false)
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	if result == nil {
		t.Fatal("ExecuteAction returned nil result")
	}
	if result.Action == nil {
		t.Fatal("result.Action is nil")
	}
	if result.Output != "Linked 2 files" {
		t.Errorf("result.Output = %q, want %q", result.Output, "Linked 2 files")
	}

	// Verify executor was called
	executor.mu.Lock()
	linkCalls := executor.linkCalls
	executor.mu.Unlock()

	if linkCalls != 1 {
		t.Errorf("executor.Link called %d times, want 1", linkCalls)
	}
}

func TestExecuteActionDedupe(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{
		groupOutput: &fclones.GroupOutput{
			Header: fclones.Header{Stats: fclones.Stats{}},
			Groups: []fclones.Group{
				{FileLen: 1000, FileHash: "hash1", Files: []string{"/a", "/b"}},
			},
		},
		dedupeOut: "Deduped 1 file",
	}
	scanner := NewScanner(database, executor, 5*time.Minute)

	// Create a scan run
	cfg := &ScanConfig{Paths: []string{"/tmp"}}
	run, _ := scanner.StartScan(context.Background(), cfg, nil)
	time.Sleep(200 * time.Millisecond)

	groups, _ := database.ListDuplicateGroups(run.ID, "")
	if len(groups) == 0 {
		t.Fatal("no groups found")
	}

	// Execute reflink/dedupe action
	result, err := scanner.ExecuteAction(context.Background(), run.ID, []int64{groups[0].ID}, db.ActionTypeReflink, false)
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	if result.Output != "Deduped 1 file" {
		t.Errorf("result.Output = %q, want %q", result.Output, "Deduped 1 file")
	}

	executor.mu.Lock()
	dedupeCalls := executor.dedupeCalls
	executor.mu.Unlock()

	if dedupeCalls != 1 {
		t.Errorf("executor.Dedupe called %d times, want 1", dedupeCalls)
	}
}

func TestExecuteActionDryRun(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{
		groupOutput: &fclones.GroupOutput{
			Header: fclones.Header{Stats: fclones.Stats{}},
			Groups: []fclones.Group{
				{FileLen: 1000, FileHash: "hash1", Files: []string{"/a", "/b"}},
			},
		},
		linkOutput: "Would link 1 file",
	}
	scanner := NewScanner(database, executor, 5*time.Minute)

	cfg := &ScanConfig{Paths: []string{"/tmp"}}
	run, _ := scanner.StartScan(context.Background(), cfg, nil)
	time.Sleep(200 * time.Millisecond)

	groups, _ := database.ListDuplicateGroups(run.ID, "")
	if len(groups) == 0 {
		t.Fatal("no groups found")
	}

	// Execute dry run
	_, err := scanner.ExecuteAction(context.Background(), run.ID, []int64{groups[0].ID}, db.ActionTypeHardlink, true)
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	// Verify group status NOT changed for dry run
	updatedGroup, _ := database.GetDuplicateGroup(groups[0].ID)
	if updatedGroup.Status != db.DuplicateGroupStatusPending {
		t.Errorf("group status = %s, want %s (should not change for dry run)",
			updatedGroup.Status, db.DuplicateGroupStatusPending)
	}
}

func TestBroadcast(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{}
	scanner := NewScanner(database, executor, 5*time.Minute)

	runID := int64(999)

	// Add subscribers
	ch1 := scanner.Subscribe(runID)
	ch2 := scanner.Subscribe(runID)

	// Broadcast progress
	progress := &types.ScanProgress{
		FilesScanned: 100,
		BytesScanned: 1000,
		Status:       "running",
	}

	go scanner.broadcast(runID, progress)

	// Both subscribers should receive the progress
	select {
	case received := <-ch1:
		if received.FilesScanned != 100 {
			t.Errorf("ch1 received FilesScanned = %d, want 100", received.FilesScanned)
		}
	case <-time.After(time.Second):
		t.Error("ch1 did not receive progress")
	}

	select {
	case received := <-ch2:
		if received.FilesScanned != 100 {
			t.Errorf("ch2 received FilesScanned = %d, want 100", received.FilesScanned)
		}
	case <-time.After(time.Second):
		t.Error("ch2 did not receive progress")
	}
}

func TestCloseSubscribers(t *testing.T) {
	database := testDB(t)
	executor := &mockExecutor{}
	scanner := NewScanner(database, executor, 5*time.Minute)

	runID := int64(888)

	ch1 := scanner.Subscribe(runID)
	ch2 := scanner.Subscribe(runID)

	// Close all subscribers
	scanner.closeSubscribers(runID)

	// Channels should be closed
	_, ok1 := <-ch1
	_, ok2 := <-ch2

	if ok1 {
		t.Error("ch1 should be closed")
	}
	if ok2 {
		t.Error("ch2 should be closed")
	}

	// Subscribers map should be cleared
	scanner.subMu.RLock()
	subs := scanner.subscribers[runID]
	scanner.subMu.RUnlock()

	if len(subs) != 0 {
		t.Errorf("expected 0 subscribers after close, got %d", len(subs))
	}
}

func TestScanConfigOptions(t *testing.T) {
	database := testDB(t)

	executor := &mockExecutor{
		groupOutput: &fclones.GroupOutput{
			Header: fclones.Header{Stats: fclones.Stats{}},
		},
	}

	// Replace Group to capture options with synchronization
	capturingExecutor := &capturingMockExecutor{
		mockExecutor: executor,
		captured:     make(chan fclones.ScanOptions, 1),
	}

	scanner := NewScanner(database, capturingExecutor, 5*time.Minute)

	maxSize := int64(10000)
	maxDepth := 5
	cfg := &ScanConfig{
		Paths:           []string{"/home", "/tmp"},
		MinSize:         1024,
		MaxSize:         &maxSize,
		IncludePatterns: []string{"*.txt", "*.doc"},
		ExcludePatterns: []string{"*.bak"},
		IncludeHidden:   true,
		FollowLinks:     true,
		OneFileSystem:   true,
		NoIgnore:        true,
		IgnoreCase:      true,
		MaxDepth:        &maxDepth,
	}

	_, err := scanner.StartScan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("StartScan failed: %v", err)
	}

	// Wait for captured options via channel (synchronizes with goroutine)
	var capturedOpts fclones.ScanOptions
	select {
	case capturedOpts = <-capturingExecutor.captured:
		// Got options
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scan to capture options")
	}

	// Verify all options were passed
	if len(capturedOpts.Paths) != 2 {
		t.Errorf("Paths = %v, want 2 paths", capturedOpts.Paths)
	}
	if capturedOpts.MinSize != 1024 {
		t.Errorf("MinSize = %d, want 1024", capturedOpts.MinSize)
	}
	if capturedOpts.MaxSize == nil || *capturedOpts.MaxSize != 10000 {
		t.Errorf("MaxSize = %v, want 10000", capturedOpts.MaxSize)
	}
	if !capturedOpts.IncludeHidden {
		t.Error("IncludeHidden should be true")
	}
	if !capturedOpts.FollowLinks {
		t.Error("FollowLinks should be true")
	}
	if !capturedOpts.OneFileSystem {
		t.Error("OneFileSystem should be true")
	}
	if !capturedOpts.NoIgnore {
		t.Error("NoIgnore should be true")
	}
	if !capturedOpts.IgnoreCase {
		t.Error("IgnoreCase should be true")
	}
	if capturedOpts.MaxDepth == nil || *capturedOpts.MaxDepth != 5 {
		t.Errorf("MaxDepth = %v, want 5", capturedOpts.MaxDepth)
	}
}

// capturingMockExecutor captures ScanOptions for verification via channel
type capturingMockExecutor struct {
	*mockExecutor
	captured chan fclones.ScanOptions
}

func (c *capturingMockExecutor) Group(ctx context.Context, opts fclones.ScanOptions, progressChan chan<- fclones.Progress) (*fclones.GroupOutput, error) {
	// Send captured options through channel (non-blocking since buffer=1)
	select {
	case c.captured <- opts:
	default:
	}
	return c.mockExecutor.Group(ctx, opts, progressChan)
}
