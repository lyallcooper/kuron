package fclones

import "context"

// ExecutorInterface defines the interface for fclones operations.
// This allows mocking the executor in tests.
type ExecutorInterface interface {
	// CheckInstalled verifies that fclones is installed and accessible
	CheckInstalled(ctx context.Context) error

	// Version returns the fclones version string
	Version(ctx context.Context) (string, error)

	// Group runs fclones group to find duplicate files
	Group(ctx context.Context, opts ScanOptions, progressChan chan<- Progress) (*GroupOutput, error)

	// GroupToInput converts groups to fclones input format for link/dedupe operations
	GroupToInput(groups []Group) string

	// Link runs fclones link to hardlink duplicate files
	Link(ctx context.Context, input string, opts LinkOptions) (string, error)

	// Dedupe runs fclones dedupe to reflink duplicate files
	Dedupe(ctx context.Context, input string, opts DedupeOptions) (string, error)
}

// Ensure Executor implements ExecutorInterface
var _ ExecutorInterface = (*Executor)(nil)
