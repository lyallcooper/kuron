package services

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/lyall/kuron/internal/db"
	"github.com/lyall/kuron/internal/fclones"
	"github.com/lyall/kuron/internal/types"
)

// Scanner orchestrates scan operations
type Scanner struct {
	db       *db.DB
	executor *fclones.Executor

	// Active scans and their cancellation functions
	mu          sync.RWMutex
	activeScans map[int64]context.CancelFunc

	// SSE subscribers
	subMu       sync.RWMutex
	subscribers map[int64][]chan *types.ScanProgress
}

// NewScanner creates a new scanner service
func NewScanner(database *db.DB, executor *fclones.Executor) *Scanner {
	return &Scanner{
		db:          database,
		executor:    executor,
		activeScans: make(map[int64]context.CancelFunc),
		subscribers: make(map[int64][]chan *types.ScanProgress),
	}
}

// Subscribe subscribes to progress updates for a scan
func (s *Scanner) Subscribe(runID int64) chan *types.ScanProgress {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	ch := make(chan *types.ScanProgress, 10)
	s.subscribers[runID] = append(s.subscribers[runID], ch)
	return ch
}

// Unsubscribe removes a subscriber
func (s *Scanner) Unsubscribe(runID int64, ch chan *types.ScanProgress) {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	subs := s.subscribers[runID]
	for i, sub := range subs {
		if sub == ch {
			s.subscribers[runID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}

	// Clean up if no more subscribers
	if len(s.subscribers[runID]) == 0 {
		delete(s.subscribers, runID)
	}
}

// broadcast sends progress to all subscribers
func (s *Scanner) broadcast(runID int64, progress *types.ScanProgress) {
	s.subMu.RLock()
	subs := s.subscribers[runID]
	s.subMu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- progress:
		default:
			// Skip if channel is full
		}
	}
}

// closeSubscribers closes all subscriber channels for a scan
func (s *Scanner) closeSubscribers(runID int64) {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	for _, ch := range s.subscribers[runID] {
		close(ch)
	}
	delete(s.subscribers, runID)
}

// StartScan starts a new scan
func (s *Scanner) StartScan(ctx context.Context, paths []string, jobID *int64) (*db.ScanRun, error) {
	// Create scan run record
	run, err := s.db.CreateScanRun(nil, jobID)
	if err != nil {
		return nil, err
	}

	// Create cancellable context
	scanCtx, cancel := context.WithCancel(context.Background())

	s.mu.Lock()
	s.activeScans[run.ID] = cancel
	s.mu.Unlock()

	// Run scan in background
	go s.runScan(scanCtx, run.ID, paths)

	return run, nil
}

// runScan executes the actual scan
func (s *Scanner) runScan(ctx context.Context, runID int64, paths []string) {
	defer func() {
		s.mu.Lock()
		delete(s.activeScans, runID)
		s.mu.Unlock()
		s.closeSubscribers(runID)
	}()

	// Progress channel
	progressChan := make(chan fclones.Progress, 100)
	defer close(progressChan)

	// Listen for progress updates
	go func() {
		for progress := range progressChan {
			s.db.UpdateScanRunProgress(runID,
				progress.FilesScanned,
				progress.BytesScanned,
				progress.GroupsFound,
				progress.FilesMatched,
				progress.WastedBytes,
			)
			s.broadcast(runID, &types.ScanProgress{
				FilesScanned: progress.FilesScanned,
				BytesScanned: progress.BytesScanned,
				GroupsFound:  progress.GroupsFound,
				WastedBytes:  progress.WastedBytes,
				Status:       "running",
			})
		}
	}()

	// Run fclones
	opts := fclones.ScanOptions{
		Paths:   paths,
		MinSize: 1, // 1 byte minimum
	}

	result, err := s.executor.Group(ctx, opts, progressChan)
	if err != nil {
		// Check if cancelled
		if ctx.Err() != nil {
			errMsg := "Scan cancelled"
			s.db.CompleteScanRun(runID, db.ScanRunStatusCancelled, &errMsg)
			s.broadcast(runID, &types.ScanProgress{Status: "cancelled"})
			return
		}

		errMsg := err.Error()
		s.db.CompleteScanRun(runID, db.ScanRunStatusFailed, &errMsg)
		s.broadcast(runID, &types.ScanProgress{Status: "failed"})
		return
	}

	// Store duplicate groups
	for _, group := range result.Groups {
		if len(group.Files) < 2 {
			continue
		}

		var files []string
		for _, f := range group.Files {
			files = append(files, f.Path)
		}

		wastedBytes := group.FileLen * int64(len(group.Files)-1)

		dg := &db.DuplicateGroup{
			ScanRunID:   runID,
			FileHash:    group.FileHash.String(),
			FileSize:    group.FileLen,
			FileCount:   len(group.Files),
			WastedBytes: wastedBytes,
			Status:      db.DuplicateGroupStatusPending,
			Files:       files,
		}
		s.db.CreateDuplicateGroup(dg)
	}

	// Update final stats
	s.db.UpdateScanRunProgress(runID,
		result.Stats.FilesTotal,
		result.Stats.BytesTotal,
		result.Stats.GroupsTotal,
		result.Stats.FilesRedundant,
		result.Stats.BytesRedundant,
	)

	// Mark complete
	s.db.CompleteScanRun(runID, db.ScanRunStatusCompleted, nil)
	s.broadcast(runID, &types.ScanProgress{
		FilesScanned: result.Stats.FilesTotal,
		BytesScanned: result.Stats.BytesTotal,
		GroupsFound:  result.Stats.GroupsTotal,
		WastedBytes:  result.Stats.BytesRedundant,
		Status:       "completed",
	})

	// Update daily stats
	// s.db.UpdateDailyStats(time.Now(), 1, int(result.Stats.GroupsTotal), int(result.Stats.FilesRedundant), result.Stats.BytesRedundant, 0)
}

// CancelScan cancels an active scan
func (s *Scanner) CancelScan(runID int64) {
	s.mu.RLock()
	cancel, ok := s.activeScans[runID]
	s.mu.RUnlock()

	if ok {
		cancel()
	}
}

// ExecuteAction executes a dedupe action on selected groups
func (s *Scanner) ExecuteAction(ctx context.Context, runID int64, groupIDs []int64, actionType db.ActionType, dryRun bool) (*db.Action, error) {
	// Create action record
	action := &db.Action{
		ScanRunID:  runID,
		ActionType: actionType,
		DryRun:     dryRun,
	}
	action, err := s.db.CreateAction(action)
	if err != nil {
		return nil, err
	}

	// Get groups
	var groups []fclones.Group
	for _, gid := range groupIDs {
		g, err := s.db.GetDuplicateGroup(gid)
		if err != nil {
			continue
		}

		var files []fclones.File
		for _, f := range g.Files {
			files = append(files, fclones.File{Path: f})
		}

		groups = append(groups, fclones.Group{
			FileLen:  g.FileSize,
			FileHash: fclones.Hash{Blake3: g.FileHash},
			Files:    files,
		})
	}

	// Convert to fclones input format
	input := s.executor.GroupToInput(groups)

	var output string
	if actionType == db.ActionTypeHardlink {
		output, err = s.executor.Link(ctx, input, fclones.LinkOptions{DryRun: dryRun})
	} else {
		output, err = s.executor.Dedupe(ctx, input, fclones.DedupeOptions{DryRun: dryRun})
	}

	if err != nil {
		errMsg := err.Error() + "\n" + output
		s.db.CompleteAction(action.ID, len(groupIDs), 0, 0, db.ActionStatusFailed, &errMsg)
		return action, err
	}

	// Calculate bytes saved
	var bytesSaved int64
	var filesProcessed int
	for _, gid := range groupIDs {
		g, err := s.db.GetDuplicateGroup(gid)
		if err != nil {
			continue
		}
		bytesSaved += g.WastedBytes
		filesProcessed += g.FileCount - 1
	}

	// Mark groups as processed (unless dry run)
	if !dryRun {
		s.db.UpdateDuplicateGroupStatus(groupIDs, db.DuplicateGroupStatusProcessed)
	}

	s.db.CompleteAction(action.ID, len(groupIDs), filesProcessed, bytesSaved, db.ActionStatusCompleted, nil)

	return action, nil
}

// ScanConfig holds configuration for a scan
type ScanConfig struct {
	Paths           []string
	MinSize         int64
	MaxSize         *int64
	IncludePatterns []string
	ExcludePatterns []string
}

// ScanConfigFromDB converts a database config to a scan config
func ScanConfigFromDB(cfg *db.ScanConfig, database *db.DB) (*ScanConfig, error) {
	var paths []string
	for _, pathID := range cfg.Paths {
		path, err := database.GetScanPath(pathID)
		if err != nil {
			continue
		}
		paths = append(paths, path.Path)
	}

	return &ScanConfig{
		Paths:           paths,
		MinSize:         cfg.MinSize,
		MaxSize:         cfg.MaxSize,
		IncludePatterns: cfg.IncludePatterns,
		ExcludePatterns: cfg.ExcludePatterns,
	}, nil
}

// GroupOutputToJSON converts group output to JSON for debugging
func GroupOutputToJSON(output *fclones.GroupOutput) string {
	data, _ := json.MarshalIndent(output, "", "  ")
	return string(data)
}
