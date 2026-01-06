package fclones

// GroupOutput represents the JSON output from fclones group command
type GroupOutput struct {
	Header Header  `json:"header"`
	Groups []Group `json:"groups"`
}

// Header contains metadata about the scan
type Header struct {
	Version   string   `json:"version"`
	Timestamp string   `json:"timestamp"`
	Command   []string `json:"command"`
	BaseDir   string   `json:"base_dir"`
	Stats     Stats    `json:"stats"`
}

// Group represents a group of duplicate files
type Group struct {
	FileLen  int64    `json:"file_len"`
	FileHash string   `json:"file_hash"`
	Files    []string `json:"files"`
}

// File represents a single file in a duplicate group (for building input)
type File struct {
	Path string
}

// Stats contains statistics from the scan
type Stats struct {
	GroupCount         int64 `json:"group_count"`
	TotalFileCount     int64 `json:"total_file_count"`
	TotalFileSize      int64 `json:"total_file_size"`
	RedundantFileCount int64 `json:"redundant_file_count"`
	RedundantFileSize  int64 `json:"redundant_file_size"`
	MissingFileCount   int64 `json:"missing_file_count"`
	MissingFileSize    int64 `json:"missing_file_size"`
}

// Hash represents a file hash (kept for compatibility)
type Hash struct {
	Blake3 string `json:"blake3,omitempty"`
	Md5    string `json:"md5,omitempty"`
	Sha1   string `json:"sha1,omitempty"`
	Sha256 string `json:"sha256,omitempty"`
}

// String returns a string representation of the hash
func (h Hash) String() string {
	if h.Blake3 != "" {
		return h.Blake3
	}
	if h.Sha256 != "" {
		return h.Sha256
	}
	if h.Sha1 != "" {
		return h.Sha1
	}
	if h.Md5 != "" {
		return h.Md5
	}
	return ""
}

// ScanOptions configures a scan operation
type ScanOptions struct {
	Paths           []string
	MinSize         int64    // Minimum file size in bytes
	MaxSize         *int64   // Maximum file size (nil = no limit)
	IncludePatterns []string // Glob patterns to include
	ExcludePatterns []string // Glob patterns to exclude
	HashFunction    string   // blake3, sha256, etc.
}

// LinkOptions configures a link operation
type LinkOptions struct {
	DryRun bool
	Soft   bool // Use symlinks instead of hardlinks
}

// DedupeOptions configures a dedupe (reflink) operation
type DedupeOptions struct {
	DryRun bool
}

// Progress represents scan progress
type Progress struct {
	Phase        string // "scanning", "filtering", "grouping", "hashing"
	FilesScanned int64
	BytesScanned int64
	GroupsFound  int64
	FilesMatched int64
	WastedBytes  int64

	// Progress bar info (from lines like "4/6: Grouping by prefix [...] 12027 / 60000")
	PhaseNum     int     // Current phase number (e.g., 4)
	PhaseTotal   int     // Total phases (e.g., 6)
	PhaseName    string  // Phase description (e.g., "Grouping by prefix")
	PhasePercent float64 // Progress within current phase (0-100)
}
