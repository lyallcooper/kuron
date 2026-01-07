package db

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// testDB creates a temporary database for testing
func testDB(t *testing.T) *DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	return db
}

// ============================================================================
// ScanRun Tests
// ============================================================================

func TestScanRun_BasicFields(t *testing.T) {
	db := testDB(t)

	// Create a scan run with basic fields
	paths := []string{"/tmp/test", "/home/user"}
	opts := &ScanRunOptions{
		MinSize: 1024,
	}

	created, err := db.CreateScanRun(nil, nil, paths, opts)
	if err != nil {
		t.Fatalf("CreateScanRun failed: %v", err)
	}

	// Test single-row query (GetScanRun uses scanScanRun)
	got, err := db.GetScanRun(created.ID)
	if err != nil {
		t.Fatalf("GetScanRun failed: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %d, want %d", got.ID, created.ID)
	}
	if !reflect.DeepEqual(got.Paths, paths) {
		t.Errorf("Paths mismatch: got %v, want %v", got.Paths, paths)
	}
	if got.MinSize != 1024 {
		t.Errorf("MinSize mismatch: got %d, want 1024", got.MinSize)
	}
	if got.Status != ScanRunStatusRunning {
		t.Errorf("Status mismatch: got %s, want %s", got.Status, ScanRunStatusRunning)
	}
}

func TestScanRun_NullableFields(t *testing.T) {
	db := testDB(t)

	maxSize := int64(1048576)
	maxDepth := 5
	opts := &ScanRunOptions{
		MinSize:         512,
		MaxSize:         &maxSize,
		IncludePatterns: []string{"*.txt", "*.doc"},
		ExcludePatterns: []string{"*.tmp"},
		IncludeHidden:   true,
		FollowLinks:     true,
		OneFileSystem:   true,
		NoIgnore:        true,
		IgnoreCase:      true,
		MaxDepth:        &maxDepth,
	}

	created, err := db.CreateScanRun(nil, nil, []string{"/test"}, opts)
	if err != nil {
		t.Fatalf("CreateScanRun failed: %v", err)
	}

	got, err := db.GetScanRun(created.ID)
	if err != nil {
		t.Fatalf("GetScanRun failed: %v", err)
	}

	// Check nullable fields are properly set
	if got.MaxSize == nil {
		t.Error("MaxSize should not be nil")
	} else if *got.MaxSize != maxSize {
		t.Errorf("MaxSize mismatch: got %d, want %d", *got.MaxSize, maxSize)
	}

	if got.MaxDepth == nil {
		t.Error("MaxDepth should not be nil")
	} else if *got.MaxDepth != maxDepth {
		t.Errorf("MaxDepth mismatch: got %d, want %d", *got.MaxDepth, maxDepth)
	}

	// Check JSON arrays
	if !reflect.DeepEqual(got.IncludePatterns, opts.IncludePatterns) {
		t.Errorf("IncludePatterns mismatch: got %v, want %v", got.IncludePatterns, opts.IncludePatterns)
	}
	if !reflect.DeepEqual(got.ExcludePatterns, opts.ExcludePatterns) {
		t.Errorf("ExcludePatterns mismatch: got %v, want %v", got.ExcludePatterns, opts.ExcludePatterns)
	}

	// Check boolean fields
	if !got.IncludeHidden {
		t.Error("IncludeHidden should be true")
	}
	if !got.FollowLinks {
		t.Error("FollowLinks should be true")
	}
	if !got.OneFileSystem {
		t.Error("OneFileSystem should be true")
	}
	if !got.NoIgnore {
		t.Error("NoIgnore should be true")
	}
	if !got.IgnoreCase {
		t.Error("IgnoreCase should be true")
	}
}

func TestScanRun_NullFieldsRemainNull(t *testing.T) {
	db := testDB(t)

	// Create with minimal options (all nullable fields null)
	created, err := db.CreateScanRun(nil, nil, []string{"/test"}, nil)
	if err != nil {
		t.Fatalf("CreateScanRun failed: %v", err)
	}

	got, err := db.GetScanRun(created.ID)
	if err != nil {
		t.Fatalf("GetScanRun failed: %v", err)
	}

	if got.ScanConfigID != nil {
		t.Errorf("ScanConfigID should be nil, got %v", got.ScanConfigID)
	}
	if got.ScheduledJobID != nil {
		t.Errorf("ScheduledJobID should be nil, got %v", got.ScheduledJobID)
	}
	if got.MaxSize != nil {
		t.Errorf("MaxSize should be nil, got %v", got.MaxSize)
	}
	if got.MaxDepth != nil {
		t.Errorf("MaxDepth should be nil, got %v", got.MaxDepth)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt should be nil, got %v", got.CompletedAt)
	}
	if got.ErrorMessage != nil {
		t.Errorf("ErrorMessage should be nil, got %v", got.ErrorMessage)
	}
}

func TestScanRun_EmptyArrays(t *testing.T) {
	db := testDB(t)

	opts := &ScanRunOptions{
		IncludePatterns: []string{},
		ExcludePatterns: []string{},
	}

	created, err := db.CreateScanRun(nil, nil, []string{"/test"}, opts)
	if err != nil {
		t.Fatalf("CreateScanRun failed: %v", err)
	}

	got, err := db.GetScanRun(created.ID)
	if err != nil {
		t.Fatalf("GetScanRun failed: %v", err)
	}

	// Empty arrays should be returned as empty (not nil)
	if got.IncludePatterns == nil {
		t.Error("IncludePatterns should be empty slice, not nil")
	}
	if len(got.IncludePatterns) != 0 {
		t.Errorf("IncludePatterns should be empty, got %v", got.IncludePatterns)
	}
}

func TestScanRun_ListUsesRowsScanner(t *testing.T) {
	db := testDB(t)

	// Create multiple scan runs
	paths1 := []string{"/path1"}
	paths2 := []string{"/path2", "/path2b"}
	paths3 := []string{"/path3"}

	_, err := db.CreateScanRun(nil, nil, paths1, nil)
	if err != nil {
		t.Fatalf("CreateScanRun 1 failed: %v", err)
	}

	_, err = db.CreateScanRun(nil, nil, paths2, &ScanRunOptions{MinSize: 100})
	if err != nil {
		t.Fatalf("CreateScanRun 2 failed: %v", err)
	}

	_, err = db.CreateScanRun(nil, nil, paths3, &ScanRunOptions{MinSize: 200})
	if err != nil {
		t.Fatalf("CreateScanRun 3 failed: %v", err)
	}

	// ListScanRuns uses scanScanRunRow internally
	runs, err := db.ListScanRuns(10, 0)
	if err != nil {
		t.Fatalf("ListScanRuns failed: %v", err)
	}

	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}

	// Verify each run was scanned correctly (order is DESC by started_at)
	// Check that paths and options are correctly preserved
	foundPaths := make(map[string]bool)
	for _, run := range runs {
		if len(run.Paths) > 0 {
			foundPaths[run.Paths[0]] = true
		}
	}

	if !foundPaths["/path1"] || !foundPaths["/path2"] || !foundPaths["/path3"] {
		t.Errorf("Not all paths found in results: %v", foundPaths)
	}
}

func TestScanRun_CompletedWithError(t *testing.T) {
	db := testDB(t)

	created, err := db.CreateScanRun(nil, nil, []string{"/test"}, nil)
	if err != nil {
		t.Fatalf("CreateScanRun failed: %v", err)
	}

	// Complete the scan with an error
	errMsg := "scan failed: permission denied"
	err = db.CompleteScanRun(created.ID, ScanRunStatusFailed, &errMsg)
	if err != nil {
		t.Fatalf("CompleteScanRun failed: %v", err)
	}

	got, err := db.GetScanRun(created.ID)
	if err != nil {
		t.Fatalf("GetScanRun failed: %v", err)
	}

	if got.Status != ScanRunStatusFailed {
		t.Errorf("Status mismatch: got %s, want %s", got.Status, ScanRunStatusFailed)
	}
	if got.ErrorMessage == nil {
		t.Error("ErrorMessage should not be nil")
	} else if *got.ErrorMessage != errMsg {
		t.Errorf("ErrorMessage mismatch: got %s, want %s", *got.ErrorMessage, errMsg)
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt should not be nil after completion")
	}
}

// ============================================================================
// DuplicateGroup Tests
// ============================================================================

func TestDuplicateGroup_BasicFields(t *testing.T) {
	db := testDB(t)

	// First create a scan run (required for foreign key)
	run, err := db.CreateScanRun(nil, nil, []string{"/test"}, nil)
	if err != nil {
		t.Fatalf("CreateScanRun failed: %v", err)
	}

	files := []string{"/test/file1.txt", "/test/file2.txt", "/test/file3.txt"}
	group := &DuplicateGroup{
		ScanRunID:   run.ID,
		FileHash:    "abc123def456",
		FileSize:    1024,
		FileCount:   3,
		WastedBytes: 2048, // (3-1) * 1024
		Status:      DuplicateGroupStatusPending,
		Files:       files,
	}

	created, err := db.CreateDuplicateGroup(group)
	if err != nil {
		t.Fatalf("CreateDuplicateGroup failed: %v", err)
	}

	// Test single-row query (GetDuplicateGroup uses scanDuplicateGroup)
	got, err := db.GetDuplicateGroup(created.ID)
	if err != nil {
		t.Fatalf("GetDuplicateGroup failed: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %d, want %d", got.ID, created.ID)
	}
	if got.ScanRunID != run.ID {
		t.Errorf("ScanRunID mismatch: got %d, want %d", got.ScanRunID, run.ID)
	}
	if got.FileHash != "abc123def456" {
		t.Errorf("FileHash mismatch: got %s, want abc123def456", got.FileHash)
	}
	if got.FileSize != 1024 {
		t.Errorf("FileSize mismatch: got %d, want 1024", got.FileSize)
	}
	if got.FileCount != 3 {
		t.Errorf("FileCount mismatch: got %d, want 3", got.FileCount)
	}
	if got.WastedBytes != 2048 {
		t.Errorf("WastedBytes mismatch: got %d, want 2048", got.WastedBytes)
	}
	if got.Status != DuplicateGroupStatusPending {
		t.Errorf("Status mismatch: got %s, want %s", got.Status, DuplicateGroupStatusPending)
	}
	if !reflect.DeepEqual(got.Files, files) {
		t.Errorf("Files mismatch: got %v, want %v", got.Files, files)
	}
}

func TestDuplicateGroup_ListUsesRowsScanner(t *testing.T) {
	db := testDB(t)

	run, err := db.CreateScanRun(nil, nil, []string{"/test"}, nil)
	if err != nil {
		t.Fatalf("CreateScanRun failed: %v", err)
	}

	// Create multiple groups with different statuses
	groups := []*DuplicateGroup{
		{
			ScanRunID:   run.ID,
			FileHash:    "hash1",
			FileSize:    1000,
			FileCount:   2,
			WastedBytes: 1000,
			Status:      DuplicateGroupStatusPending,
			Files:       []string{"/a.txt", "/b.txt"},
		},
		{
			ScanRunID:   run.ID,
			FileHash:    "hash2",
			FileSize:    2000,
			FileCount:   3,
			WastedBytes: 4000,
			Status:      DuplicateGroupStatusProcessed,
			Files:       []string{"/c.txt", "/d.txt", "/e.txt"},
		},
		{
			ScanRunID:   run.ID,
			FileHash:    "hash3",
			FileSize:    500,
			FileCount:   2,
			WastedBytes: 500,
			Status:      DuplicateGroupStatusIgnored,
			Files:       []string{"/f.txt", "/g.txt"},
		},
	}

	for _, g := range groups {
		_, err := db.CreateDuplicateGroup(g)
		if err != nil {
			t.Fatalf("CreateDuplicateGroup failed: %v", err)
		}
	}

	// ListDuplicateGroups uses scanDuplicateGroupRow internally
	got, err := db.ListDuplicateGroups(run.ID, "")
	if err != nil {
		t.Fatalf("ListDuplicateGroups failed: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(got))
	}

	// Verify hashes are all present
	hashes := make(map[string]bool)
	for _, g := range got {
		hashes[g.FileHash] = true
	}
	if !hashes["hash1"] || !hashes["hash2"] || !hashes["hash3"] {
		t.Errorf("Not all hashes found: %v", hashes)
	}
}

func TestDuplicateGroup_FilesWithSpecialCharacters(t *testing.T) {
	db := testDB(t)

	run, err := db.CreateScanRun(nil, nil, []string{"/test"}, nil)
	if err != nil {
		t.Fatalf("CreateScanRun failed: %v", err)
	}

	// Files with special characters that need proper JSON encoding
	files := []string{
		"/path/with spaces/file.txt",
		"/path/with\"quotes\"/file.txt",
		"/path/with\ttab/file.txt",
		"/path/with/unicode/文件.txt",
	}

	group := &DuplicateGroup{
		ScanRunID:   run.ID,
		FileHash:    "specialhash",
		FileSize:    100,
		FileCount:   len(files),
		WastedBytes: 300,
		Status:      DuplicateGroupStatusPending,
		Files:       files,
	}

	created, err := db.CreateDuplicateGroup(group)
	if err != nil {
		t.Fatalf("CreateDuplicateGroup failed: %v", err)
	}

	got, err := db.GetDuplicateGroup(created.ID)
	if err != nil {
		t.Fatalf("GetDuplicateGroup failed: %v", err)
	}

	if !reflect.DeepEqual(got.Files, files) {
		t.Errorf("Files with special characters not preserved:\ngot:  %v\nwant: %v", got.Files, files)
	}
}

// ============================================================================
// ScheduledJob Tests
// ============================================================================

func TestScheduledJob_BasicFields(t *testing.T) {
	db := testDB(t)

	nextRun := time.Now().Add(time.Hour).Truncate(time.Second)
	job := &ScheduledJob{
		Name:           "Test Job",
		Paths:          []string{"/home/user/documents"},
		MinSize:        1024,
		CronExpression: "0 2 * * *",
		Action:         "scan",
		Enabled:        true,
		NextRunAt:      &nextRun,
	}

	created, err := db.CreateScheduledJob(job)
	if err != nil {
		t.Fatalf("CreateScheduledJob failed: %v", err)
	}

	// Test single-row query (GetScheduledJob uses scanScheduledJob)
	got, err := db.GetScheduledJob(created.ID)
	if err != nil {
		t.Fatalf("GetScheduledJob failed: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %d, want %d", got.ID, created.ID)
	}
	if got.Name != "Test Job" {
		t.Errorf("Name mismatch: got %s, want Test Job", got.Name)
	}
	if !reflect.DeepEqual(got.Paths, job.Paths) {
		t.Errorf("Paths mismatch: got %v, want %v", got.Paths, job.Paths)
	}
	if got.MinSize != 1024 {
		t.Errorf("MinSize mismatch: got %d, want 1024", got.MinSize)
	}
	if got.CronExpression != "0 2 * * *" {
		t.Errorf("CronExpression mismatch: got %s, want 0 2 * * *", got.CronExpression)
	}
	if got.Action != "scan" {
		t.Errorf("Action mismatch: got %s, want scan", got.Action)
	}
	if !got.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestScheduledJob_AllNullableFields(t *testing.T) {
	db := testDB(t)

	maxSize := int64(10485760)
	maxDepth := 10
	nextRun := time.Now().Add(time.Hour).Truncate(time.Second)

	job := &ScheduledJob{
		Name:            "Full Job",
		Paths:           []string{"/path1", "/path2"},
		MinSize:         100,
		MaxSize:         &maxSize,
		IncludePatterns: []string{"*.go", "*.rs"},
		ExcludePatterns: []string{"*_test.go", "vendor/*"},
		CronExpression:  "0 0 * * 0",
		Action:          "scan_hardlink",
		Enabled:         true,
		NextRunAt:       &nextRun,
		IncludeHidden:   true,
		FollowLinks:     true,
		OneFileSystem:   true,
		NoIgnore:        true,
		IgnoreCase:      true,
		MaxDepth:        &maxDepth,
	}

	created, err := db.CreateScheduledJob(job)
	if err != nil {
		t.Fatalf("CreateScheduledJob failed: %v", err)
	}

	got, err := db.GetScheduledJob(created.ID)
	if err != nil {
		t.Fatalf("GetScheduledJob failed: %v", err)
	}

	// Check nullable fields
	if got.MaxSize == nil {
		t.Error("MaxSize should not be nil")
	} else if *got.MaxSize != maxSize {
		t.Errorf("MaxSize mismatch: got %d, want %d", *got.MaxSize, maxSize)
	}

	if got.MaxDepth == nil {
		t.Error("MaxDepth should not be nil")
	} else if *got.MaxDepth != maxDepth {
		t.Errorf("MaxDepth mismatch: got %d, want %d", *got.MaxDepth, maxDepth)
	}

	// Check JSON arrays
	if !reflect.DeepEqual(got.IncludePatterns, job.IncludePatterns) {
		t.Errorf("IncludePatterns mismatch: got %v, want %v", got.IncludePatterns, job.IncludePatterns)
	}
	if !reflect.DeepEqual(got.ExcludePatterns, job.ExcludePatterns) {
		t.Errorf("ExcludePatterns mismatch: got %v, want %v", got.ExcludePatterns, job.ExcludePatterns)
	}

	// Check boolean flags
	if !got.IncludeHidden {
		t.Error("IncludeHidden should be true")
	}
	if !got.FollowLinks {
		t.Error("FollowLinks should be true")
	}
	if !got.OneFileSystem {
		t.Error("OneFileSystem should be true")
	}
	if !got.NoIgnore {
		t.Error("NoIgnore should be true")
	}
	if !got.IgnoreCase {
		t.Error("IgnoreCase should be true")
	}
}

func TestScheduledJob_NullFieldsRemainNull(t *testing.T) {
	db := testDB(t)

	job := &ScheduledJob{
		Name:           "Minimal Job",
		Paths:          []string{"/test"},
		CronExpression: "0 * * * *",
		Action:         "scan",
		Enabled:        false,
	}

	created, err := db.CreateScheduledJob(job)
	if err != nil {
		t.Fatalf("CreateScheduledJob failed: %v", err)
	}

	got, err := db.GetScheduledJob(created.ID)
	if err != nil {
		t.Fatalf("GetScheduledJob failed: %v", err)
	}

	if got.MaxSize != nil {
		t.Errorf("MaxSize should be nil, got %v", got.MaxSize)
	}
	if got.MaxDepth != nil {
		t.Errorf("MaxDepth should be nil, got %v", got.MaxDepth)
	}
	if got.LastRunAt != nil {
		t.Errorf("LastRunAt should be nil, got %v", got.LastRunAt)
	}
}

func TestScheduledJob_ListUsesRowsScanner(t *testing.T) {
	db := testDB(t)

	jobs := []*ScheduledJob{
		{
			Name:           "Alpha Job",
			Paths:          []string{"/alpha"},
			CronExpression: "0 1 * * *",
			Action:         "scan",
			Enabled:        true,
		},
		{
			Name:           "Beta Job",
			Paths:          []string{"/beta"},
			CronExpression: "0 2 * * *",
			Action:         "scan_hardlink",
			Enabled:        false,
		},
		{
			Name:           "Gamma Job",
			Paths:          []string{"/gamma"},
			CronExpression: "0 3 * * *",
			Action:         "scan_reflink",
			Enabled:        true,
		},
	}

	for _, j := range jobs {
		_, err := db.CreateScheduledJob(j)
		if err != nil {
			t.Fatalf("CreateScheduledJob failed: %v", err)
		}
	}

	// ListScheduledJobs uses scanScheduledJobRow internally
	got, err := db.ListScheduledJobs()
	if err != nil {
		t.Fatalf("ListScheduledJobs failed: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(got))
	}

	// Results are ordered by name
	if got[0].Name != "Alpha Job" {
		t.Errorf("First job should be Alpha Job, got %s", got[0].Name)
	}
	if got[1].Name != "Beta Job" {
		t.Errorf("Second job should be Beta Job, got %s", got[1].Name)
	}
	if got[2].Name != "Gamma Job" {
		t.Errorf("Third job should be Gamma Job, got %s", got[2].Name)
	}
}

func TestScheduledJob_UpdatePreservesFields(t *testing.T) {
	db := testDB(t)

	maxSize := int64(5000)
	maxDepth := 3
	job := &ScheduledJob{
		Name:            "Update Test",
		Paths:           []string{"/original"},
		MinSize:         100,
		MaxSize:         &maxSize,
		IncludePatterns: []string{"*.txt"},
		ExcludePatterns: []string{"*.log"},
		CronExpression:  "0 0 * * *",
		Action:          "scan",
		Enabled:         true,
		IncludeHidden:   true,
		MaxDepth:        &maxDepth,
	}

	created, err := db.CreateScheduledJob(job)
	if err != nil {
		t.Fatalf("CreateScheduledJob failed: %v", err)
	}

	// Update the job
	newMaxSize := int64(10000)
	created.Paths = []string{"/updated", "/new"}
	created.MaxSize = &newMaxSize
	created.IncludePatterns = []string{"*.md", "*.txt"}
	created.Enabled = false

	err = db.UpdateScheduledJob(created)
	if err != nil {
		t.Fatalf("UpdateScheduledJob failed: %v", err)
	}

	got, err := db.GetScheduledJob(created.ID)
	if err != nil {
		t.Fatalf("GetScheduledJob failed: %v", err)
	}

	if !reflect.DeepEqual(got.Paths, []string{"/updated", "/new"}) {
		t.Errorf("Paths not updated: got %v", got.Paths)
	}
	if got.MaxSize == nil || *got.MaxSize != 10000 {
		t.Errorf("MaxSize not updated: got %v", got.MaxSize)
	}
	if !reflect.DeepEqual(got.IncludePatterns, []string{"*.md", "*.txt"}) {
		t.Errorf("IncludePatterns not updated: got %v", got.IncludePatterns)
	}
	if got.Enabled {
		t.Error("Enabled should be false")
	}

	// Fields that weren't changed should be preserved
	if got.MinSize != 100 {
		t.Errorf("MinSize changed unexpectedly: got %d", got.MinSize)
	}
	if !got.IncludeHidden {
		t.Error("IncludeHidden should still be true")
	}
}

// ============================================================================
// Cross-function consistency tests
// These ensure that the Row and Rows scanners produce identical results
// ============================================================================

func TestScanRun_RowAndRowsConsistency(t *testing.T) {
	db := testDB(t)

	maxSize := int64(999)
	maxDepth := 7
	opts := &ScanRunOptions{
		MinSize:         42,
		MaxSize:         &maxSize,
		IncludePatterns: []string{"*.a", "*.b"},
		ExcludePatterns: []string{"*.x"},
		IncludeHidden:   true,
		FollowLinks:     false,
		OneFileSystem:   true,
		NoIgnore:        false,
		IgnoreCase:      true,
		MaxDepth:        &maxDepth,
	}

	created, err := db.CreateScanRun(nil, nil, []string{"/consistency"}, opts)
	if err != nil {
		t.Fatalf("CreateScanRun failed: %v", err)
	}

	// Get via single-row scanner
	single, err := db.GetScanRun(created.ID)
	if err != nil {
		t.Fatalf("GetScanRun failed: %v", err)
	}

	// Get via multi-row scanner
	list, err := db.ListScanRuns(10, 0)
	if err != nil {
		t.Fatalf("ListScanRuns failed: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 run, got %d", len(list))
	}
	multi := list[0]

	// Compare all fields
	if single.ID != multi.ID {
		t.Error("ID mismatch")
	}
	if !reflect.DeepEqual(single.Paths, multi.Paths) {
		t.Error("Paths mismatch")
	}
	if single.MinSize != multi.MinSize {
		t.Error("MinSize mismatch")
	}
	if (single.MaxSize == nil) != (multi.MaxSize == nil) {
		t.Error("MaxSize nil mismatch")
	} else if single.MaxSize != nil && *single.MaxSize != *multi.MaxSize {
		t.Error("MaxSize value mismatch")
	}
	if !reflect.DeepEqual(single.IncludePatterns, multi.IncludePatterns) {
		t.Error("IncludePatterns mismatch")
	}
	if !reflect.DeepEqual(single.ExcludePatterns, multi.ExcludePatterns) {
		t.Error("ExcludePatterns mismatch")
	}
	if single.IncludeHidden != multi.IncludeHidden {
		t.Error("IncludeHidden mismatch")
	}
	if single.FollowLinks != multi.FollowLinks {
		t.Error("FollowLinks mismatch")
	}
	if single.OneFileSystem != multi.OneFileSystem {
		t.Error("OneFileSystem mismatch")
	}
	if single.NoIgnore != multi.NoIgnore {
		t.Error("NoIgnore mismatch")
	}
	if single.IgnoreCase != multi.IgnoreCase {
		t.Error("IgnoreCase mismatch")
	}
	if (single.MaxDepth == nil) != (multi.MaxDepth == nil) {
		t.Error("MaxDepth nil mismatch")
	} else if single.MaxDepth != nil && *single.MaxDepth != *multi.MaxDepth {
		t.Error("MaxDepth value mismatch")
	}
}

func TestDuplicateGroup_RowAndRowsConsistency(t *testing.T) {
	db := testDB(t)

	run, _ := db.CreateScanRun(nil, nil, []string{"/test"}, nil)

	group := &DuplicateGroup{
		ScanRunID:   run.ID,
		FileHash:    "consistencyhash",
		FileSize:    12345,
		FileCount:   5,
		WastedBytes: 49380,
		Status:      DuplicateGroupStatusPending,
		Files:       []string{"/a", "/b", "/c", "/d", "/e"},
	}

	created, err := db.CreateDuplicateGroup(group)
	if err != nil {
		t.Fatalf("CreateDuplicateGroup failed: %v", err)
	}

	// Get via single-row scanner
	single, err := db.GetDuplicateGroup(created.ID)
	if err != nil {
		t.Fatalf("GetDuplicateGroup failed: %v", err)
	}

	// Get via multi-row scanner
	list, err := db.ListDuplicateGroups(run.ID, "")
	if err != nil {
		t.Fatalf("ListDuplicateGroups failed: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 group, got %d", len(list))
	}
	multi := list[0]

	// Compare all fields
	if single.ID != multi.ID {
		t.Error("ID mismatch")
	}
	if single.ScanRunID != multi.ScanRunID {
		t.Error("ScanRunID mismatch")
	}
	if single.FileHash != multi.FileHash {
		t.Error("FileHash mismatch")
	}
	if single.FileSize != multi.FileSize {
		t.Error("FileSize mismatch")
	}
	if single.FileCount != multi.FileCount {
		t.Error("FileCount mismatch")
	}
	if single.WastedBytes != multi.WastedBytes {
		t.Error("WastedBytes mismatch")
	}
	if single.Status != multi.Status {
		t.Error("Status mismatch")
	}
	if !reflect.DeepEqual(single.Files, multi.Files) {
		t.Error("Files mismatch")
	}
}

func TestScheduledJob_RowAndRowsConsistency(t *testing.T) {
	db := testDB(t)

	maxSize := int64(8888)
	maxDepth := 4
	nextRun := time.Now().Add(time.Hour).Truncate(time.Second)

	job := &ScheduledJob{
		Name:            "Consistency Job",
		Paths:           []string{"/cons1", "/cons2"},
		MinSize:         256,
		MaxSize:         &maxSize,
		IncludePatterns: []string{"*.cons"},
		ExcludePatterns: []string{"skip.*"},
		CronExpression:  "30 4 * * *",
		Action:          "scan_reflink",
		Enabled:         true,
		NextRunAt:       &nextRun,
		IncludeHidden:   true,
		FollowLinks:     true,
		OneFileSystem:   false,
		NoIgnore:        true,
		IgnoreCase:      false,
		MaxDepth:        &maxDepth,
	}

	created, err := db.CreateScheduledJob(job)
	if err != nil {
		t.Fatalf("CreateScheduledJob failed: %v", err)
	}

	// Get via single-row scanner
	single, err := db.GetScheduledJob(created.ID)
	if err != nil {
		t.Fatalf("GetScheduledJob failed: %v", err)
	}

	// Get via multi-row scanner
	list, err := db.ListScheduledJobs()
	if err != nil {
		t.Fatalf("ListScheduledJobs failed: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 job, got %d", len(list))
	}
	multi := list[0]

	// Compare all fields
	if single.ID != multi.ID {
		t.Error("ID mismatch")
	}
	if single.Name != multi.Name {
		t.Error("Name mismatch")
	}
	if !reflect.DeepEqual(single.Paths, multi.Paths) {
		t.Error("Paths mismatch")
	}
	if single.MinSize != multi.MinSize {
		t.Error("MinSize mismatch")
	}
	if (single.MaxSize == nil) != (multi.MaxSize == nil) {
		t.Error("MaxSize nil mismatch")
	} else if single.MaxSize != nil && *single.MaxSize != *multi.MaxSize {
		t.Error("MaxSize value mismatch")
	}
	if !reflect.DeepEqual(single.IncludePatterns, multi.IncludePatterns) {
		t.Error("IncludePatterns mismatch")
	}
	if !reflect.DeepEqual(single.ExcludePatterns, multi.ExcludePatterns) {
		t.Error("ExcludePatterns mismatch")
	}
	if single.CronExpression != multi.CronExpression {
		t.Error("CronExpression mismatch")
	}
	if single.Action != multi.Action {
		t.Error("Action mismatch")
	}
	if single.Enabled != multi.Enabled {
		t.Error("Enabled mismatch")
	}
	if single.IncludeHidden != multi.IncludeHidden {
		t.Error("IncludeHidden mismatch")
	}
	if single.FollowLinks != multi.FollowLinks {
		t.Error("FollowLinks mismatch")
	}
	if single.OneFileSystem != multi.OneFileSystem {
		t.Error("OneFileSystem mismatch")
	}
	if single.NoIgnore != multi.NoIgnore {
		t.Error("NoIgnore mismatch")
	}
	if single.IgnoreCase != multi.IgnoreCase {
		t.Error("IgnoreCase mismatch")
	}
	if (single.MaxDepth == nil) != (multi.MaxDepth == nil) {
		t.Error("MaxDepth nil mismatch")
	} else if single.MaxDepth != nil && *single.MaxDepth != *multi.MaxDepth {
		t.Error("MaxDepth value mismatch")
	}
}

// ============================================================================
// Additional Database Tests
// ============================================================================

func TestGetEnabledJobs(t *testing.T) {
	db := testDB(t)

	// Create enabled job with next run time
	nextRun := time.Now().Add(-time.Hour) // Past time (should trigger)
	enabledJob := &ScheduledJob{
		Name:           "Enabled",
		Paths:          []string{"/test"},
		CronExpression: "0 * * * *",
		Action:         "scan",
		Enabled:        true,
		NextRunAt:      &nextRun,
	}
	db.CreateScheduledJob(enabledJob)

	// Create disabled job
	disabledJob := &ScheduledJob{
		Name:           "Disabled",
		Paths:          []string{"/test"},
		CronExpression: "0 * * * *",
		Action:         "scan",
		Enabled:        false,
		NextRunAt:      &nextRun,
	}
	db.CreateScheduledJob(disabledJob)

	// Create enabled job without next run time
	enabledNoNext := &ScheduledJob{
		Name:           "Enabled No Next",
		Paths:          []string{"/test"},
		CronExpression: "0 * * * *",
		Action:         "scan",
		Enabled:        true,
		NextRunAt:      nil,
	}
	db.CreateScheduledJob(enabledNoNext)

	jobs, err := db.GetEnabledJobs()
	if err != nil {
		t.Fatalf("GetEnabledJobs failed: %v", err)
	}

	// Should return only enabled jobs (2 of them)
	if len(jobs) != 2 {
		t.Errorf("expected 2 enabled jobs, got %d", len(jobs))
	}

	// Verify disabled job is not included
	for _, j := range jobs {
		if j.Name == "Disabled" {
			t.Error("disabled job should not be in GetEnabledJobs result")
		}
	}
}

func TestCleanupOldData(t *testing.T) {
	db := testDB(t)

	// Create old scan run (should be deleted)
	oldRun, _ := db.CreateScanRun(nil, nil, []string{"/old"}, nil)
	db.CompleteScanRun(oldRun.ID, ScanRunStatusCompleted, nil)

	// Manually backdate the completed_at using embedded *sql.DB
	_, err := db.Exec(`UPDATE scan_runs SET completed_at = datetime('now', '-60 days') WHERE id = ?`, oldRun.ID)
	if err != nil {
		t.Fatalf("failed to backdate scan run: %v", err)
	}

	// Create recent scan run (should be kept)
	recentRun, _ := db.CreateScanRun(nil, nil, []string{"/recent"}, nil)
	db.CompleteScanRun(recentRun.ID, ScanRunStatusCompleted, nil)

	// Create old action (should be deleted)
	oldAction := &Action{
		ScanRunID:  oldRun.ID,
		ActionType: ActionTypeHardlink,
	}
	db.CreateAction(oldAction)

	// Run cleanup with 30 day retention
	err = db.CleanupOldData(30)
	if err != nil {
		t.Fatalf("CleanupOldData failed: %v", err)
	}

	// Verify old run was deleted
	_, err = db.GetScanRun(oldRun.ID)
	if err == nil {
		t.Error("old scan run should have been deleted")
	}

	// Verify recent run still exists
	_, err = db.GetScanRun(recentRun.ID)
	if err != nil {
		t.Error("recent scan run should still exist")
	}
}

func TestPagination(t *testing.T) {
	db := testDB(t)

	// Create 5 scan runs
	for i := 0; i < 5; i++ {
		db.CreateScanRun(nil, nil, []string{"/test"}, nil)
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	tests := []struct {
		name      string
		limit     int
		offset    int
		wantCount int
	}{
		{"first page", 2, 0, 2},
		{"second page", 2, 2, 2},
		{"last page (partial)", 2, 4, 1},
		{"offset beyond count", 2, 10, 0},
		{"large limit", 100, 0, 5},
		// Note: LIMIT 0 returns 0 rows in SQL, not "all"
		{"zero limit returns zero", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runs, err := db.ListScanRuns(tt.limit, tt.offset)
			if err != nil {
				t.Fatalf("ListScanRuns failed: %v", err)
			}
			if len(runs) != tt.wantCount {
				t.Errorf("got %d runs, want %d", len(runs), tt.wantCount)
			}
		})
	}
}

func TestDuplicateGroupPaginatedSorting(t *testing.T) {
	db := testDB(t)

	run, _ := db.CreateScanRun(nil, nil, []string{"/test"}, nil)

	// Create groups with different sizes
	groups := []*DuplicateGroup{
		{ScanRunID: run.ID, FileHash: "a", FileSize: 1000, FileCount: 2, WastedBytes: 1000, Status: DuplicateGroupStatusPending, Files: []string{"/a", "/b"}},
		{ScanRunID: run.ID, FileHash: "b", FileSize: 3000, FileCount: 5, WastedBytes: 12000, Status: DuplicateGroupStatusPending, Files: []string{"/c", "/d"}},
		{ScanRunID: run.ID, FileHash: "c", FileSize: 2000, FileCount: 3, WastedBytes: 4000, Status: DuplicateGroupStatusPending, Files: []string{"/e", "/f"}},
	}

	for _, g := range groups {
		db.CreateDuplicateGroup(g)
	}

	// Test sorting by wasted (default, DESC)
	result, err := db.ListDuplicateGroupsPaginated(DuplicateGroupQuery{
		ScanRunID: run.ID,
		SortBy:    "wasted",
		SortOrder: "desc",
	})
	if err != nil {
		t.Fatalf("ListDuplicateGroupsPaginated failed: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(result))
	}

	// Highest wasted should be first
	if result[0].WastedBytes != 12000 {
		t.Errorf("first group should have WastedBytes=12000, got %d", result[0].WastedBytes)
	}

	// Test sorting by count
	result, err = db.ListDuplicateGroupsPaginated(DuplicateGroupQuery{
		ScanRunID: run.ID,
		SortBy:    "count",
		SortOrder: "desc",
	})
	if err != nil {
		t.Fatalf("ListDuplicateGroupsPaginated failed: %v", err)
	}

	// Highest count should be first
	if result[0].FileCount != 5 {
		t.Errorf("first group should have FileCount=5, got %d", result[0].FileCount)
	}
}

func TestCountDuplicateGroups(t *testing.T) {
	db := testDB(t)

	run, _ := db.CreateScanRun(nil, nil, []string{"/test"}, nil)

	// Create groups with different statuses
	db.CreateDuplicateGroup(&DuplicateGroup{ScanRunID: run.ID, FileHash: "a", FileSize: 100, FileCount: 2, WastedBytes: 100, Status: DuplicateGroupStatusPending, Files: []string{"/a", "/b"}})
	db.CreateDuplicateGroup(&DuplicateGroup{ScanRunID: run.ID, FileHash: "b", FileSize: 100, FileCount: 2, WastedBytes: 100, Status: DuplicateGroupStatusPending, Files: []string{"/c", "/d"}})
	db.CreateDuplicateGroup(&DuplicateGroup{ScanRunID: run.ID, FileHash: "c", FileSize: 100, FileCount: 2, WastedBytes: 100, Status: DuplicateGroupStatusProcessed, Files: []string{"/e", "/f"}})

	// Count all
	count, err := db.CountDuplicateGroups(run.ID, "")
	if err != nil {
		t.Fatalf("CountDuplicateGroups failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 total groups, got %d", count)
	}

	// Count pending only
	count, err = db.CountDuplicateGroups(run.ID, "pending")
	if err != nil {
		t.Fatalf("CountDuplicateGroups failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 pending groups, got %d", count)
	}

	// Count processed only
	count, err = db.CountDuplicateGroups(run.ID, "processed")
	if err != nil {
		t.Fatalf("CountDuplicateGroups failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 processed group, got %d", count)
	}
}

func TestUpdateDuplicateGroupStatus(t *testing.T) {
	db := testDB(t)

	run, _ := db.CreateScanRun(nil, nil, []string{"/test"}, nil)

	g1, _ := db.CreateDuplicateGroup(&DuplicateGroup{ScanRunID: run.ID, FileHash: "a", FileSize: 100, FileCount: 2, WastedBytes: 100, Status: DuplicateGroupStatusPending, Files: []string{"/a", "/b"}})
	g2, _ := db.CreateDuplicateGroup(&DuplicateGroup{ScanRunID: run.ID, FileHash: "b", FileSize: 100, FileCount: 2, WastedBytes: 100, Status: DuplicateGroupStatusPending, Files: []string{"/c", "/d"}})
	g3, _ := db.CreateDuplicateGroup(&DuplicateGroup{ScanRunID: run.ID, FileHash: "c", FileSize: 100, FileCount: 2, WastedBytes: 100, Status: DuplicateGroupStatusPending, Files: []string{"/e", "/f"}})

	// Update status for g1 and g2 only
	err := db.UpdateDuplicateGroupStatus([]int64{g1.ID, g2.ID}, DuplicateGroupStatusProcessed)
	if err != nil {
		t.Fatalf("UpdateDuplicateGroupStatus failed: %v", err)
	}

	// Verify g1 and g2 are updated
	got1, _ := db.GetDuplicateGroup(g1.ID)
	if got1.Status != DuplicateGroupStatusProcessed {
		t.Errorf("g1 status = %s, want processed", got1.Status)
	}

	got2, _ := db.GetDuplicateGroup(g2.ID)
	if got2.Status != DuplicateGroupStatusProcessed {
		t.Errorf("g2 status = %s, want processed", got2.Status)
	}

	// Verify g3 is unchanged
	got3, _ := db.GetDuplicateGroup(g3.ID)
	if got3.Status != DuplicateGroupStatusPending {
		t.Errorf("g3 status = %s, want pending (should be unchanged)", got3.Status)
	}
}

func TestDeleteScheduledJob(t *testing.T) {
	db := testDB(t)

	job := &ScheduledJob{
		Name:           "To Delete",
		Paths:          []string{"/test"},
		CronExpression: "0 * * * *",
		Action:         "scan",
		Enabled:        true,
	}
	created, _ := db.CreateScheduledJob(job)

	err := db.DeleteScheduledJob(created.ID)
	if err != nil {
		t.Fatalf("DeleteScheduledJob failed: %v", err)
	}

	_, err = db.GetScheduledJob(created.ID)
	if err == nil {
		t.Error("job should be deleted")
	}
}

func TestGetDashboardStats(t *testing.T) {
	db := testDB(t)

	// Create scan runs with groups
	run1, _ := db.CreateScanRun(nil, nil, []string{"/test"}, nil)
	db.CompleteScanRun(run1.ID, ScanRunStatusCompleted, nil)
	db.CreateDuplicateGroup(&DuplicateGroup{ScanRunID: run1.ID, FileHash: "a", FileSize: 1000, FileCount: 2, WastedBytes: 1000, Status: DuplicateGroupStatusPending, Files: []string{"/a", "/b"}})
	db.CreateDuplicateGroup(&DuplicateGroup{ScanRunID: run1.ID, FileHash: "b", FileSize: 2000, FileCount: 3, WastedBytes: 4000, Status: DuplicateGroupStatusProcessed, Files: []string{"/c", "/d", "/e"}})

	// Create action to record saved bytes
	action, _ := db.CreateAction(&Action{ScanRunID: run1.ID, ActionType: ActionTypeHardlink})
	db.CompleteAction(action.ID, 1, 2, 4000, ActionStatusCompleted, nil)

	totalSaved, pendingGroups, recentScans, err := db.GetDashboardStats()
	if err != nil {
		t.Fatalf("GetDashboardStats failed: %v", err)
	}

	if totalSaved != 4000 {
		t.Errorf("totalSaved = %d, want 4000", totalSaved)
	}
	if pendingGroups != 1 {
		t.Errorf("pendingGroups = %d, want 1", pendingGroups)
	}
	if recentScans != 1 {
		t.Errorf("recentScans = %d, want 1", recentScans)
	}
}
