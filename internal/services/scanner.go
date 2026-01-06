package services

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/lyallcooper/kuron/internal/db"
	"github.com/lyallcooper/kuron/internal/fclones"
	"github.com/lyallcooper/kuron/internal/types"
)

// subscriber wraps a channel with safe close handling
type subscriber struct {
	ch        chan *types.ScanProgress
	closeOnce sync.Once
	closed    bool
}

func (sub *subscriber) close() {
	sub.closeOnce.Do(func() {
		sub.closed = true
		close(sub.ch)
	})
}

func (sub *subscriber) send(progress *types.ScanProgress) bool {
	if sub.closed {
		return false
	}
	select {
	case sub.ch <- progress:
		return true
	default:
		return false
	}
}

// Scanner orchestrates scan operations
type Scanner struct {
	db          *db.DB
	executor    *fclones.Executor
	scanTimeout time.Duration

	// Active scans and their cancellation functions
	mu          sync.RWMutex
	activeScans map[int64]context.CancelFunc

	// SSE subscribers
	subMu       sync.RWMutex
	subscribers map[int64][]*subscriber
}

// NewScanner creates a new scanner service
func NewScanner(database *db.DB, executor *fclones.Executor, scanTimeout time.Duration) *Scanner {
	return &Scanner{
		db:          database,
		executor:    executor,
		scanTimeout: scanTimeout,
		activeScans: make(map[int64]context.CancelFunc),
		subscribers: make(map[int64][]*subscriber),
	}
}

// Subscribe subscribes to progress updates for a scan
func (s *Scanner) Subscribe(runID int64) chan *types.ScanProgress {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	sub := &subscriber{
		ch: make(chan *types.ScanProgress, 10),
	}
	s.subscribers[runID] = append(s.subscribers[runID], sub)
	return sub.ch
}

// Unsubscribe removes a subscriber
func (s *Scanner) Unsubscribe(runID int64, ch chan *types.ScanProgress) {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	subs := s.subscribers[runID]
	for i, sub := range subs {
		if sub.ch == ch {
			// Remove from slice first, then close safely
			s.subscribers[runID] = append(subs[:i], subs[i+1:]...)
			sub.close()
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
	// Make a copy of the slice to avoid holding lock during send
	subs := make([]*subscriber, len(s.subscribers[runID]))
	copy(subs, s.subscribers[runID])
	s.subMu.RUnlock()

	for _, sub := range subs {
		sub.send(progress)
	}
}

// closeSubscribers closes all subscriber channels for a scan
func (s *Scanner) closeSubscribers(runID int64) {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	for _, sub := range s.subscribers[runID] {
		sub.close()
	}
	delete(s.subscribers, runID)
}

// StartScan starts a new scan with full configuration
func (s *Scanner) StartScan(ctx context.Context, cfg *ScanConfig, jobID *int64) (*db.ScanRun, error) {
	// Create scan run record with paths
	run, err := s.db.CreateScanRun(nil, jobID, cfg.Paths)
	if err != nil {
		return nil, err
	}

	// Create context with timeout (can also be cancelled manually)
	scanCtx, cancel := context.WithTimeout(context.Background(), s.scanTimeout)

	s.mu.Lock()
	s.activeScans[run.ID] = cancel
	s.mu.Unlock()

	// Run scan in background
	go s.runScan(scanCtx, run.ID, cfg)

	return run, nil
}

// runScan executes the actual scan
func (s *Scanner) runScan(ctx context.Context, runID int64, cfg *ScanConfig) {
	startTime := time.Now()
	log.Printf("scan %d: starting scan of %s", runID, strings.Join(cfg.Paths, ", "))

	defer func() {
		s.mu.Lock()
		delete(s.activeScans, runID)
		s.mu.Unlock()
		s.closeSubscribers(runID)
	}()

	// Progress channel
	progressChan := make(chan fclones.Progress, 100)

	// Track last progress for final stats (files_scanned from progress != files in duplicate groups from JSON)
	var lastProgress fclones.Progress
	var progressMu sync.Mutex
	progressDone := make(chan struct{})

	// Listen for progress updates
	go func() {
		defer close(progressDone)
		for progress := range progressChan {
			progressMu.Lock()
			lastProgress = progress
			progressMu.Unlock()
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
				PhaseNum:     progress.PhaseNum,
				PhaseTotal:   progress.PhaseTotal,
				PhaseName:    progress.PhaseName,
				PhasePercent: progress.PhasePercent,
			})
		}
	}()

	// Run fclones with full config
	opts := fclones.ScanOptions{
		Paths:           cfg.Paths,
		MinSize:         cfg.MinSize,
		MaxSize:         cfg.MaxSize,
		IncludePatterns: cfg.IncludePatterns,
		ExcludePatterns: cfg.ExcludePatterns,
		IncludeHidden:   cfg.IncludeHidden,
		FollowLinks:     cfg.FollowLinks,
		OneFileSystem:   cfg.OneFileSystem,
		NoIgnore:        cfg.NoIgnore,
		IgnoreCase:      cfg.IgnoreCase,
		MaxDepth:        cfg.MaxDepth,
	}

	result, err := s.executor.Group(ctx, opts, progressChan)

	// Close progress channel and wait for goroutine to finish processing
	close(progressChan)
	<-progressDone

	// Get final progress values
	progressMu.Lock()
	filesScanned := lastProgress.FilesScanned
	bytesScanned := lastProgress.BytesScanned
	progressMu.Unlock()

	if err != nil {
		// Check if cancelled
		if ctx.Err() != nil {
			errMsg := "Scan cancelled"
			s.db.CompleteScanRun(runID, db.ScanRunStatusCancelled, &errMsg)
			s.broadcast(runID, &types.ScanProgress{Status: "cancelled"})
			log.Printf("scan %d: cancelled after %s", runID, time.Since(startTime).Round(time.Second))
			return
		}

		errMsg := err.Error()
		s.db.CompleteScanRun(runID, db.ScanRunStatusFailed, &errMsg)
		s.broadcast(runID, &types.ScanProgress{Status: "failed"})
		log.Printf("scan %d: failed after %s: %s", runID, time.Since(startTime).Round(time.Second), errMsg)
		return
	}

	// Store duplicate groups
	for _, group := range result.Groups {
		if len(group.Files) < 2 {
			continue
		}

		wastedBytes := group.FileLen * int64(len(group.Files)-1)

		dg := &db.DuplicateGroup{
			ScanRunID:   runID,
			FileHash:    group.FileHash,
			FileSize:    group.FileLen,
			FileCount:   len(group.Files),
			WastedBytes: wastedBytes,
			Status:      db.DuplicateGroupStatusPending,
			Files:       group.Files,
		}
		s.db.CreateDuplicateGroup(dg)
	}

	// Update final stats
	// Note: stats.TotalFileCount/TotalFileSize are files IN duplicate groups, not total scanned
	// We use filesScanned and bytesScanned from progress parsing for the actual values
	stats := result.Header.Stats
	s.db.UpdateScanRunProgress(runID,
		filesScanned,
		bytesScanned,
		stats.GroupCount,
		stats.RedundantFileCount,
		stats.RedundantFileSize,
	)

	// Mark complete
	s.db.CompleteScanRun(runID, db.ScanRunStatusCompleted, nil)
	s.broadcast(runID, &types.ScanProgress{
		FilesScanned: filesScanned,
		BytesScanned: bytesScanned,
		GroupsFound:  stats.GroupCount,
		WastedBytes:  stats.RedundantFileSize,
		Status:       "completed",
	})

	log.Printf("scan %d: completed in %s, found %d duplicate groups", runID, time.Since(startTime).Round(time.Second), stats.GroupCount)
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

// ActionResult contains the result of an action execution
type ActionResult struct {
	Action *db.Action
	Output string // fclones command output
}

// ExecuteAction executes a dedupe action on selected groups
func (s *Scanner) ExecuteAction(ctx context.Context, runID int64, groupIDs []int64, actionType db.ActionType, dryRun bool) (*ActionResult, error) {
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

		groups = append(groups, fclones.Group{
			FileLen:  g.FileSize,
			FileHash: g.FileHash,
			Files:    g.Files,
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
		return &ActionResult{Action: action, Output: output}, err
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

	return &ActionResult{Action: action, Output: output}, nil
}

// ScanConfig holds configuration for a scan
type ScanConfig struct {
	Paths           []string
	MinSize         int64
	MaxSize         *int64
	IncludePatterns []string
	ExcludePatterns []string

	// Advanced options
	IncludeHidden bool
	FollowLinks   bool
	OneFileSystem bool
	NoIgnore      bool
	IgnoreCase    bool
	MaxDepth      *int
}

// GroupOutputToJSON converts group output to JSON for debugging
func GroupOutputToJSON(output *fclones.GroupOutput) string {
	data, _ := json.MarshalIndent(output, "", "  ")
	return string(data)
}
