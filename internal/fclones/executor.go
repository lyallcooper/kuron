package fclones

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/creack/pty"
)

// ansiRegex matches ANSI escape codes
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\].*?\x07`)

// stripANSI removes ANSI escape codes from a string
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

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
	args := []string{"group", "--format", "json"}

	// Add size filters
	if opts.MinSize > 0 {
		args = append(args, "-s", strconv.FormatInt(opts.MinSize, 10))
	}
	if opts.MaxSize != nil {
		args = append(args, "--max-size", strconv.FormatInt(*opts.MaxSize, 10))
	}

	// Add include patterns
	for _, pattern := range opts.IncludePatterns {
		args = append(args, "--name", pattern)
	}

	// Add exclude patterns
	for _, pattern := range opts.ExcludePatterns {
		args = append(args, "--exclude", pattern)
	}

	// Add hash function if specified
	if opts.HashFunction != "" {
		args = append(args, "--hash-fn", opts.HashFunction)
	}

	// Add paths
	args = append(args, opts.Paths...)

	cmd := exec.CommandContext(ctx, e.binaryPath, args...)

	// Use pty to trick fclones into showing progress bar
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start fclones: %w", err)
	}
	defer ptmx.Close()

	// Read from pty and separate progress lines from JSON output
	// Progress/log lines start with '[' (timestamp) or digit (progress bar like "6/6:")
	// JSON output starts with '{'
	var jsonBuf bytes.Buffer
	inJSON := false
	var progress Progress

	scanner := bufio.NewScanner(ptmx)
	scanner.Split(scanLinesOrCR)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Strip ANSI escape codes for clean parsing
		cleaned := stripANSI(line)
		trimmed := strings.TrimSpace(cleaned)
		if trimmed == "" {
			continue
		}

		// Detect start of JSON output
		// JSON starts with '{' - could be on its own line or at end of a progress line
		// Progress lines never contain '{', so the first '{' marks the start of JSON
		if !inJSON {
			if idx := strings.Index(cleaned, "{"); idx != -1 {
				inJSON = true
				jsonBuf.WriteString(cleaned[idx:] + "\n")
				// Parse any progress text before the JSON started
				if idx > 0 {
					e.parseProgressLine(cleaned[:idx], &progress, progressChan)
				}
				continue
			}
		}

		if inJSON {
			jsonBuf.WriteString(cleaned + "\n")
		} else {
			// Parse progress line and send update
			e.parseProgressLine(cleaned, &progress, progressChan)
		}
	}

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

	data, _ := json.Marshal(output)
	return string(data)
}

// scanLinesOrCR is a custom split function for bufio.Scanner that splits on
// both \n and \r. This is needed because fclones uses \r for progress bar updates.
func scanLinesOrCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[0:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
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
	if err != nil {
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

// parseProgressBar parses a progress bar line like "4/6: Grouping by prefix [...] 12027 / 60000"
func parseProgressBar(line string) *progressBarInfo {
	// Find phase number pattern "N/M:"
	colonIdx := strings.Index(line, ":")
	if colonIdx == -1 || colonIdx < 3 {
		return nil
	}

	phaseStr := strings.TrimSpace(line[:colonIdx])
	slashIdx := strings.Index(phaseStr, "/")
	if slashIdx == -1 {
		return nil
	}

	phaseNum, err1 := strconv.Atoi(phaseStr[:slashIdx])
	phaseTotal, err2 := strconv.Atoi(phaseStr[slashIdx+1:])
	if err1 != nil || err2 != nil {
		return nil
	}

	// Extract phase name (between ":" and "[")
	remainder := line[colonIdx+1:]
	bracketIdx := strings.Index(remainder, "[")
	if bracketIdx == -1 {
		return nil
	}
	phaseName := strings.TrimSpace(remainder[:bracketIdx])

	// Find the progress values after "]"
	closeBracketIdx := strings.LastIndex(remainder, "]")
	if closeBracketIdx == -1 {
		return nil
	}

	progressStr := strings.TrimSpace(remainder[closeBracketIdx+1:])
	// Parse "12027 / 60000" or "4.0 GB / 59.3 GB"
	slashIdx = strings.Index(progressStr, " / ")
	if slashIdx == -1 {
		return nil
	}

	currentStr := strings.TrimSpace(progressStr[:slashIdx])
	totalStr := strings.TrimSpace(progressStr[slashIdx+3:])

	// Try parsing as bytes first (handles "4.0 GB" format)
	current := parseBytes(currentStr)
	total := parseBytes(totalStr)

	// If parseBytes returns 0 for both, try parsing as plain integers
	if current == 0 && total == 0 {
		if c, err := strconv.ParseInt(currentStr, 10, 64); err == nil {
			current = c
		}
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

// parseProgressLine parses a single line of fclones output and updates progress
func (e *Executor) parseProgressLine(line string, progress *Progress, progressChan chan<- Progress) {
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
		select {
		case progressChan <- *progress:
		default:
			// Don't block if channel is full
		}
	}
}
