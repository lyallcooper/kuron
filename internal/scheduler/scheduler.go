package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/lyall/kuron/internal/db"
	"github.com/lyall/kuron/internal/services"
	"github.com/robfig/cron/v3"
)

// Scheduler manages scheduled jobs
type Scheduler struct {
	db      *db.DB
	scanner *services.Scanner
	parser  cron.Parser

	mu       sync.RWMutex
	running  bool
	stopChan chan struct{}
	cancel   context.CancelFunc // Cancel function for running jobs
	wg       sync.WaitGroup     // Tracks spawned job goroutines
}

// New creates a new scheduler
func New(database *db.DB, scanner *services.Scanner) *Scheduler {
	return &Scheduler{
		db:      database,
		scanner: scanner,
		parser:  cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

// Start starts the scheduler
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopChan = make(chan struct{})

	// Create cancellable context for all spawned jobs
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()

	go s.run(ctx)
}

// Stop stops the scheduler and waits for running jobs to complete
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)

	// Cancel all running job contexts
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()

	// Wait for all spawned job goroutines to finish
	s.wg.Wait()
}

// run is the main scheduler loop
func (s *Scheduler) run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	// Check immediately on start
	s.checkJobs(ctx)

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.checkJobs(ctx)
		}
	}
}

// checkJobs checks for jobs that need to run
func (s *Scheduler) checkJobs(ctx context.Context) {
	jobs, err := s.db.GetEnabledJobs()
	if err != nil {
		log.Printf("scheduler: failed to get jobs: %v", err)
		return
	}

	now := time.Now()

	for _, job := range jobs {
		if job.NextRunAt == nil {
			continue
		}

		if now.After(*job.NextRunAt) || now.Equal(*job.NextRunAt) {
			s.wg.Add(1)
			go s.runJob(ctx, job)
		}
	}
}

// runJob executes a scheduled job
func (s *Scheduler) runJob(ctx context.Context, job *db.ScheduledJob) {
	defer s.wg.Done()

	log.Printf("scheduler: running job %d (%s)", job.ID, job.Name)

	// Check if context is already cancelled
	if ctx.Err() != nil {
		log.Printf("scheduler: job %d cancelled before start", job.ID)
		return
	}

	if len(job.Paths) == 0 {
		log.Printf("scheduler: no paths configured for job %d", job.ID)
		return
	}

	// Build scan config from job
	cfg := &services.ScanConfig{
		Paths:           job.Paths,
		MinSize:         job.MinSize,
		MaxSize:         job.MaxSize,
		IncludePatterns: job.IncludePatterns,
		ExcludePatterns: job.ExcludePatterns,
	}

	// Start scan with cancellable context
	run, err := s.scanner.StartScan(ctx, cfg, &job.ID)
	if err != nil {
		log.Printf("scheduler: failed to start scan for job %d: %v", job.ID, err)
		return
	}

	// Update last run time
	now := time.Now()
	schedule, err := s.parser.Parse(job.CronExpression)
	if err != nil {
		log.Printf("scheduler: invalid cron expression for job %d: %v", job.ID, err)
		return
	}

	nextRun := schedule.Next(now)
	if err := s.db.UpdateJobLastRun(job.ID, now, nextRun); err != nil {
		log.Printf("scheduler: failed to update job last run: %v", err)
	}

	log.Printf("scheduler: started scan run %d for job %d, next run at %v", run.ID, job.ID, nextRun)

	// If action is specified, wait for scan to complete and execute action
	if job.Action != "scan" {
		s.waitAndExecuteAction(ctx, run.ID, job)
	}
}

// waitAndExecuteAction waits for a scan to complete and executes the configured action
func (s *Scheduler) waitAndExecuteAction(ctx context.Context, runID int64, job *db.ScheduledJob) {
	// Poll for completion
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, err := s.db.GetScanRun(runID)
			if err != nil {
				log.Printf("scheduler: failed to get scan run: %v", err)
				return
			}

			if run.Status == db.ScanRunStatusRunning {
				continue
			}

			if run.Status != db.ScanRunStatusCompleted {
				log.Printf("scheduler: scan %d did not complete successfully, skipping action", runID)
				return
			}

			// Get all pending groups
			groups, err := s.db.ListDuplicateGroups(runID, "pending")
			if err != nil {
				log.Printf("scheduler: failed to get duplicate groups: %v", err)
				return
			}

			if len(groups) == 0 {
				log.Printf("scheduler: no duplicate groups found for scan %d", runID)
				return
			}

			// Collect group IDs
			var groupIDs []int64
			for _, g := range groups {
				groupIDs = append(groupIDs, g.ID)
			}

			// Determine action type
			var actionType db.ActionType
			if job.Action == "scan_hardlink" {
				actionType = db.ActionTypeHardlink
			} else {
				actionType = db.ActionTypeReflink
			}

			// Execute action (not dry run for scheduled jobs)
			_, err = s.scanner.ExecuteAction(ctx, runID, groupIDs, actionType, false)
			if err != nil {
				log.Printf("scheduler: failed to execute action: %v", err)
			} else {
				log.Printf("scheduler: executed %s on %d groups for job %d", actionType, len(groupIDs), job.ID)
			}

			return
		}
	}
}

// UpdateNextRun updates the next run time for a job
func (s *Scheduler) UpdateNextRun(job *db.ScheduledJob) error {
	schedule, err := s.parser.Parse(job.CronExpression)
	if err != nil {
		return err
	}

	nextRun := schedule.Next(time.Now())
	job.NextRunAt = &nextRun

	return s.db.UpdateScheduledJob(job)
}
