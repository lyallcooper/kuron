package types

// ScanProgress represents scan progress for SSE updates
type ScanProgress struct {
	FilesScanned int64
	BytesScanned int64
	GroupsFound  int64
	WastedBytes  int64
	Status       string

	// Progress bar info
	PhaseNum     int
	PhaseTotal   int
	PhaseName    string
	PhasePercent float64
}
