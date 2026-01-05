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
	s.mu.Unlock()

	go s.run()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)
	s.mu.Unlock()
}

// run is the main scheduler loop
func (s *Scheduler) run() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	// Check immediately on start
	s.checkJobs()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.checkJobs()
		}
	}
}

// checkJobs checks for jobs that need to run
func (s *Scheduler) checkJobs() {
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
			go s.runJob(job)
		}
	}
}

// runJob executes a scheduled job
func (s *Scheduler) runJob(job *db.ScheduledJob) {
	// Get scan config
	cfg, err := s.db.GetScanConfig(job.ScanConfigID)
	if err != nil {
		log.Printf("scheduler: failed to get scan config for job %d: %v", job.ID, err)
		return
	}

	log.Printf("scheduler: running job %d (%s)", job.ID, cfg.Name)

	// Get paths
	var paths []string
	for _, pathID := range cfg.Paths {
		path, err := s.db.GetScanPath(pathID)
		if err != nil {
			continue
		}
		paths = append(paths, path.Path)
	}

	if len(paths) == 0 {
		log.Printf("scheduler: no valid paths for job %d", job.ID)
		return
	}

	// Start scan
	ctx := context.Background()
	run, err := s.scanner.StartScan(ctx, paths, &job.ID)
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
		go s.waitAndExecuteAction(ctx, run.ID, job)
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
