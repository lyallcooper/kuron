package fclones

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Executor runs fclones commands
type Executor struct {
	binaryPath string
}

// NewExecutor creates a new fclones executor
func NewExecutor() *Executor {
	return &Executor{
		binaryPath: "fclones",
	}
}

// SetBinaryPath sets a custom path to the fclones binary
func (e *Executor) SetBinaryPath(path string) {
	e.binaryPath = path
}

// CheckInstalled verifies that fclones is installed and accessible
func (e *Executor) CheckInstalled(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, e.binaryPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("fclones not found or not executable: %w", err)
	}
	if !strings.Contains(string(output), "fclones") {
		return fmt.Errorf("unexpected output from fclones --version: %s", output)
	}
	return nil
}

// Version returns the fclones version string
func (e *Executor) Version(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, e.binaryPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("fclones not found: %w", err)
	}
	// Output is typically "fclones 0.35.0" - extract just the version number
	version := strings.TrimSpace(string(output))
	if parts := strings.Fields(version); len(parts) >= 2 {
		version = parts[1]
	}
	return version, nil
}

// Group runs fclones group and returns duplicate groups
func (e *Executor) Group(ctx context.Context, opts ScanOptions, progressChan chan<- Progress) (*GroupOutput, error) {
	args := []string{"--progress=true", "group", "--format", "json"}

	// Add size filters
	if opts.MinSize > 0 {
		args = append(args, "-s", strconv.FormatInt(opts.MinSize, 10))
	}
	if opts.MaxSize != nil {
		args = append(args, "--max-size", strconv.FormatInt(*opts.MaxSize, 10))
	}

	// Add include patterns (match on full path)
	for _, pattern := range opts.IncludePatterns {
		args = append(args, "--path", pattern)
	}

	// Add exclude patterns
	for _, pattern := range opts.ExcludePatterns {
		args = append(args, "--exclude", pattern)
	}

	// Add hash function if specified
	if opts.HashFunction != "" {
		args = append(args, "--hash-fn", opts.HashFunction)
	}

	// Advanced options
	if opts.IncludeHidden {
		args = append(args, "--hidden")
	}
	if opts.FollowLinks {
		args = append(args, "--follow-links")
	}
	if opts.OneFileSystem {
		args = append(args, "--one-fs")
	}
	if opts.NoIgnore {
		args = append(args, "--no-ignore")
	}
	if opts.IgnoreCase {
		args = append(args, "--ignore-case")
	}
	if opts.MaxDepth != nil {
		args = append(args, "--depth", strconv.Itoa(*opts.MaxDepth))
	}

	// Add paths
	args = append(args, opts.Paths...)

	cmd := exec.CommandContext(ctx, e.binaryPath, args...)

	// Get stdout for JSON output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Get stderr for progress output
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start fclones: %w", err)
	}

	// Read stderr (progress) in a goroutine
	var progress Progress
	var lastSendTime time.Time
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		e.readProgress(stderr, &progress, progressChan, &lastSendTime)
	}()

	// Read stdout (JSON output)
	var jsonBuf bytes.Buffer
	io.Copy(&jsonBuf, stdout)

	// Wait for stderr reading to complete
	wg.Wait()

	// Check for context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Wait for command to finish
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("fclones exited with error: %w", err)
	}

	// Parse JSON output
	var result GroupOutput
	if err := json.Unmarshal(jsonBuf.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse fclones output: %w (got: %s)", err, jsonBuf.String()[:min(200, jsonBuf.Len())])
	}

	return &result, nil
}

// readProgress reads progress output from fclones stderr and sends updates
func (e *Executor) readProgress(r io.Reader, progress *Progress, progressChan chan<- Progress, lastSendTime *time.Time) {
	const maxLineLen = 64 * 1024 // 64KB max line length for safety

	scanner := bufio.NewScanner(r)
	// Set buffer to limit memory usage
	buf := make([]byte, 4096)
	scanner.Buffer(buf, maxLineLen)

	// Use custom split function that splits on both \r and \n for progress bar updates
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		for i, b := range data {
			if b == '\n' || b == '\r' {
				return i + 1, data[0:i], nil
			}
		}
		// If we've accumulated more than maxLineLen without finding a delimiter,
		// just return what we have to prevent unbounded memory growth
		if len(data) >= maxLineLen {
			return len(data), data, nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	})

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		e.parseProgressLine(line, progress, progressChan, lastSendTime)
	}
}

// Link runs fclones link to hardlink duplicate files
func (e *Executor) Link(ctx context.Context, input string, opts LinkOptions) (string, error) {
	args := []string{"link"}

	if opts.DryRun {
		args = append(args, "--dry-run")
	}
	if opts.Soft {
		args = append(args, "--soft")
	}

	cmd := exec.CommandContext(ctx, e.binaryPath, args...)
	cmd.Stdin = strings.NewReader(input)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("fclones link failed: %w", err)
	}

	return string(output), nil
}

// Dedupe runs fclones dedupe to create reflinks
func (e *Executor) Dedupe(ctx context.Context, input string, opts DedupeOptions) (string, error) {
	args := []string{"dedupe"}

	if opts.DryRun {
		args = append(args, "--dry-run")
	}

	cmd := exec.CommandContext(ctx, e.binaryPath, args...)
	cmd.Stdin = strings.NewReader(input)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("fclones dedupe failed: %w", err)
	}

	return string(output), nil
}

// Remove runs fclones remove to delete duplicate files
func (e *Executor) Remove(ctx context.Context, input string, opts RemoveOptions) (string, error) {
	args := []string{"remove"}

	if opts.DryRun {
		args = append(args, "--dry-run")
	}
	if opts.Priority != "" {
		args = append(args, "--priority", opts.Priority)
	}

	cmd := exec.CommandContext(ctx, e.binaryPath, args...)
	cmd.Stdin = strings.NewReader(input)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("fclones remove failed: %w", err)
	}

	return string(output), nil
}

// GroupToInput converts groups to JSON format for link/dedupe commands
func (e *Executor) GroupToInput(groups []Group) string {
	// Filter out groups with less than 2 files and calculate stats
	var validGroups []Group
	var totalFiles int64
	var totalSize int64
	var redundantFiles int64
	var redundantSize int64

	// Collect unique base paths for the command field
	pathSet := make(map[string]bool)

	for _, g := range groups {
		if len(g.Files) >= 2 {
			validGroups = append(validGroups, g)
			totalFiles += int64(len(g.Files))
			totalSize += g.FileLen * int64(len(g.Files))
			redundantFiles += int64(len(g.Files) - 1)
			redundantSize += g.FileLen * int64(len(g.Files)-1)

			// Extract base path from first file for command reconstruction
			if len(g.Files) > 0 {
				pathSet["/"] = true // Use root as fallback
			}
		}
	}

	// Build command array (fclones requires this to have valid paths)
	command := []string{"fclones", "group", "--format", "json"}
	for path := range pathSet {
		command = append(command, path)
	}

	output := GroupOutput{
		Header: Header{
			Version:   "0.35.0",
			Timestamp: time.Now().Format(time.RFC3339),
			Command:   command,
			BaseDir:   "/",
			Stats: Stats{
				GroupCount:         int64(len(validGroups)),
				TotalFileCount:     totalFiles,
				TotalFileSize:      totalSize,
				RedundantFileCount: redundantFiles,
				RedundantFileSize:  redundantSize,
			},
		},
		Groups: validGroups,
	}

	data, err := json.Marshal(output)
	if err != nil {
		log.Printf("fclones: failed to marshal groups to JSON: %v", err)
		return ""
	}
	return string(data)
}

// parseBytes converts human-readable byte strings like "4.0 GB" to int64
func parseBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Find where the number ends and the unit begins
	var numStr string
	var unit string
	for i, c := range s {
		if c == ' ' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			numStr = strings.TrimSpace(s[:i])
			unit = strings.TrimSpace(s[i:])
			break
		}
	}
	if numStr == "" {
		numStr = s
	}

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil || num < 0 {
		return 0
	}

	unit = strings.ToUpper(strings.TrimSpace(unit))
	switch unit {
	case "B", "":
		return int64(num)
	case "KB", "K":
		return int64(num * 1000)
	case "KIB":
		return int64(num * 1024)
	case "MB", "M":
		return int64(num * 1000 * 1000)
	case "MIB":
		return int64(num * 1024 * 1024)
	case "GB", "G":
		return int64(num * 1000 * 1000 * 1000)
	case "GIB":
		return int64(num * 1024 * 1024 * 1024)
	case "TB", "T":
		return int64(num * 1000 * 1000 * 1000 * 1000)
	case "TIB":
		return int64(num * 1024 * 1024 * 1024 * 1024)
	default:
		return int64(num)
	}
}

// progressBarInfo holds parsed progress bar data
type progressBarInfo struct {
	PhaseNum     int
	PhaseTotal   int
	PhaseName    string
	PhasePercent float64
}

// progressBarRegex matches progress bar patterns with current/total format:
// "6/6: Grouping by contents [...] 630.5 MB / 3.6 GB" or "4/6: Grouping by prefix [...] 12027 / 60000"
var progressBarRegex = regexp.MustCompile(`(\d)/(\d): ([^[]+)\[([^\]]*)\]\s*(\S+(?:\s+[KMGT]i?B)?)\s*/\s*(\S+(?:\s+[KMGT]i?B)?)`)

// scanningPhaseRegex matches phase 1 scanning format: "1/6: Scanning files [...] 12345" (count only, no total)
var scanningPhaseRegex = regexp.MustCompile(`(\d)/(\d): ([^[]+)\[([^\]]*)\]\s+(\d+)(?:\s|$)`)

// parseProgressBar parses the LAST progress bar from a line that may contain multiple concatenated progress bars
// Handles two formats:
// 1. "4/6: Grouping by prefix [...] 12027 / 60000" (current/total)
// 2. "1/6: Scanning files [...] 12345" (count only, no total)
func parseProgressBar(line string) *progressBarInfo {
	// First try the current/total format (phases 2-6)
	matches := progressBarRegex.FindAllStringSubmatch(line, -1)
	if len(matches) > 0 {
		// Use the last match (most recent progress)
		match := matches[len(matches)-1]

		phaseNum, _ := strconv.Atoi(match[1])
		phaseTotal, _ := strconv.Atoi(match[2])
		phaseName := strings.TrimSpace(match[3])
		currentStr := match[5]
		totalStr := match[6]

		// Parse values - could be bytes (4.0 GB) or counts (12027)
		current := parseBytes(currentStr)
		total := parseBytes(totalStr)

		// If parseBytes returns 0, try parsing as plain integers
		if current == 0 {
			if c, err := strconv.ParseInt(currentStr, 10, 64); err == nil {
				current = c
			}
		}
		if total == 0 {
			if t, err := strconv.ParseInt(totalStr, 10, 64); err == nil {
				total = t
			}
		}

		var percent float64
		if total > 0 {
			percent = float64(current) / float64(total) * 100
			if percent > 100 {
				percent = 100
			}
		}

		return &progressBarInfo{
			PhaseNum:     phaseNum,
			PhaseTotal:   phaseTotal,
			PhaseName:    phaseName,
			PhasePercent: percent,
		}
	}

	// Try the scanning format (phase 1 - count only, no total)
	scanMatches := scanningPhaseRegex.FindAllStringSubmatch(line, -1)
	if len(scanMatches) > 0 {
		match := scanMatches[len(scanMatches)-1]

		phaseNum, _ := strconv.Atoi(match[1])
		phaseTotal, _ := strconv.Atoi(match[2])
		phaseName := strings.TrimSpace(match[3])

		// For scanning phase, we don't have a total so we use -1 to indicate indeterminate
		return &progressBarInfo{
			PhaseNum:     phaseNum,
			PhaseTotal:   phaseTotal,
			PhaseName:    phaseName,
			PhasePercent: -1, // Indeterminate - we don't know the total
		}
	}

	return nil
}

// phaseNameToPhase converts a phase name to a short phase identifier
func phaseNameToPhase(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "scanning"):
		return "scanning"
	case strings.Contains(lower, "contents"):
		return "hashing"
	case strings.Contains(lower, "grouping"), strings.Contains(lower, "prefix"), strings.Contains(lower, "suffix"), strings.Contains(lower, "size"), strings.Contains(lower, "path"):
		return "grouping"
	case strings.Contains(lower, "initializing"):
		return "initializing"
	default:
		return "processing"
	}
}

// parseProgressLine parses a single line of fclones output and updates progress.
// If lastSendTime is provided, progress updates are throttled to 50ms minimum between sends.
func (e *Executor) parseProgressLine(line string, progress *Progress, progressChan chan<- Progress, lastSendTime *time.Time) {
	if progressChan == nil {
		return
	}

	// Parse fclones output formats:
	// 1. Log lines: "[timestamp] fclones:  info: Scanned 45828 file entries"
	// 2. Progress bar: "6/6: Grouping by contents [==>   ] 4.0 GB / 59.3 GB"

	updated := false

	if strings.Contains(line, "Scanned") && strings.Contains(line, "file entries") {
		// Parse: "Scanned 45828 file entries"
		parts := strings.Fields(line)
		for i, p := range parts {
			if p == "Scanned" && i+1 < len(parts) {
				if n, err := strconv.ParseInt(parts[i+1], 10, 64); err == nil {
					progress.FilesScanned = n
					updated = true
				}
			}
		}
		progress.Phase = "scanning"
	} else if strings.Contains(line, "files matching selection criteria") {
		// Parse: "Found 45466 (180.4 GB) files matching selection criteria"
		parts := strings.Fields(line)
		for i, p := range parts {
			if p == "Found" && i+1 < len(parts) {
				if n, err := strconv.ParseInt(parts[i+1], 10, 64); err == nil {
					progress.FilesMatched = n
					updated = true
				}
			}
		}
		// Extract bytes from parentheses: "(180.4 GB)"
		if start := strings.Index(line, "("); start != -1 {
			if end := strings.Index(line[start:], ")"); end != -1 {
				bytesStr := line[start+1 : start+end]
				if bytes := parseBytes(bytesStr); bytes > 0 {
					progress.BytesScanned = bytes
					updated = true
				}
			}
		}
		progress.Phase = "filtering"
	} else if strings.Contains(line, "candidates after") {
		// Parse: "Found 10047 (30.1 GB) candidates after grouping by size"
		parts := strings.Fields(line)
		for i, p := range parts {
			if p == "Found" && i+1 < len(parts) {
				if n, err := strconv.ParseInt(parts[i+1], 10, 64); err == nil {
					progress.GroupsFound = n
					updated = true
				}
			}
		}
		progress.Phase = "grouping"
	} else if strings.Contains(line, "/") && strings.Contains(line, ":") && strings.Contains(line, "[") {
		// Progress bar line: "4/6: Grouping by prefix [...] 12027 / 60000"
		// or: "6/6: Grouping by contents [...] 4.0 GB / 59.3 GB"
		if parsed := parseProgressBar(line); parsed != nil {
			progress.PhaseNum = parsed.PhaseNum
			progress.PhaseTotal = parsed.PhaseTotal
			progress.PhaseName = parsed.PhaseName
			progress.PhasePercent = parsed.PhasePercent
			progress.Phase = phaseNameToPhase(parsed.PhaseName)
			updated = true
		}
	}

	if updated {
		// Throttle progress updates if lastSendTime is provided (50ms = 20 updates/sec max)
		if lastSendTime != nil {
			now := time.Now()
			if now.Sub(*lastSendTime) < 50*time.Millisecond {
				return // Skip this update, too soon
			}
			*lastSendTime = now
		}

		select {
		case progressChan <- *progress:
		default:
			// Don't block if channel is full
		}
	}
}
