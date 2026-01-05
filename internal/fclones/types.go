package fclones

// GroupOutput represents the JSON output from fclones group command
type GroupOutput struct {
	Header Header  `json:"header"`
	Groups []Group `json:"groups"`
	Stats  Stats   `json:"stats"`
}

// Header contains metadata about the scan
type Header struct {
	Version   string   `json:"version"`
	Timestamp string   `json:"timestamp"`
	Command   string   `json:"command"`
	Paths     []string `json:"paths"`
}

// Group represents a group of duplicate files
type Group struct {
	FileLen  int64    `json:"file_len"`
	FileHash Hash     `json:"file_hash"`
	Files    []File   `json:"files"`
}

// Hash represents a file hash
type Hash struct {
	Blake3 string `json:"blake3,omitempty"`
	Md5    string `json:"md5,omitempty"`
	Sha1   string `json:"sha1,omitempty"`
	Sha256 string `json:"sha256,omitempty"`
}

// File represents a single file in a duplicate group
type File struct {
	Path string `json:"path"`
}

// Stats contains statistics from the scan
type Stats struct {
	FilesTotal     int64 `json:"files_total"`
	FilesMatched   int64 `json:"files_matched"`
	FilesRedundant int64 `json:"files_redundant"`
	BytesTotal     int64 `json:"bytes_total"`
	BytesMatched   int64 `json:"bytes_matched"`
	BytesRedundant int64 `json:"bytes_redundant"`
	GroupsTotal    int64 `json:"groups_total"`
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
	DryRun  bool
	Soft    bool // Use symlinks instead of hardlinks
}

// DedupeOptions configures a dedupe (reflink) operation
type DedupeOptions struct {
	DryRun bool
}

// Progress represents scan progress
type Progress struct {
	Phase        string // "scanning", "hashing", "grouping"
	FilesScanned int64
	BytesScanned int64
	GroupsFound  int64
	FilesMatched int64
	WastedBytes  int64
}
