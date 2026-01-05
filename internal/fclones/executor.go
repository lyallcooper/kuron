package fclones

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
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

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start fclones: %w", err)
	}

	// Read stderr for progress (fclones outputs progress to stderr)
	go e.readProgress(stderr, progressChan)

	// Read stdout for JSON output
	output, err := io.ReadAll(stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to read output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("fclones exited with error: %w", err)
	}

	var result GroupOutput
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse fclones output: %w", err)
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

// GroupToInput converts a group output to input format for link/dedupe commands
func (e *Executor) GroupToInput(groups []Group) string {
	var builder strings.Builder

	for _, g := range groups {
		if len(g.Files) < 2 {
			continue
		}

		// Write group header
		builder.WriteString(fmt.Sprintf("%d\n", g.FileLen))

		// Write file paths
		for _, f := range g.Files {
			builder.WriteString(f)
			builder.WriteString("\n")
		}

		// Empty line between groups
		builder.WriteString("\n")
	}

	return builder.String()
}

// readProgress parses fclones stderr for progress updates
func (e *Executor) readProgress(r io.Reader, progressChan chan<- Progress) {
	if progressChan == nil {
		io.Copy(io.Discard, r)
		return
	}

	scanner := bufio.NewScanner(r)
	var progress Progress

	for scanner.Scan() {
		line := scanner.Text()

		// Parse progress lines from fclones
		// fclones outputs progress like:
		// "Scanned 1234 files, 567 MB"
		// "Found 89 groups, 45 redundant files, 123 MB redundant"

		if strings.Contains(line, "Scanned") {
			// Parse scanning progress
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "Scanned" && i+1 < len(parts) {
					if n, err := strconv.ParseInt(parts[i+1], 10, 64); err == nil {
						progress.FilesScanned = n
					}
				}
			}
			progress.Phase = "scanning"
		} else if strings.Contains(line, "Found") && strings.Contains(line, "groups") {
			// Parse grouping results
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "Found" && i+1 < len(parts) {
					if n, err := strconv.ParseInt(parts[i+1], 10, 64); err == nil {
						progress.GroupsFound = n
					}
				}
				if p == "redundant" && i > 0 {
					if n, err := strconv.ParseInt(parts[i-1], 10, 64); err == nil {
						if strings.Contains(line, "files") {
							progress.FilesMatched = n
						}
					}
				}
			}
			progress.Phase = "grouping"
		}

		select {
		case progressChan <- progress:
		default:
			// Don't block if channel is full
		}
	}
}
