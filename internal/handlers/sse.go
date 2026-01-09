package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lyallcooper/kuron/internal/types"
)

// ScanProgressSSE handles SSE connections for scan progress
func (h *Handler) ScanProgressSSE(w http.ResponseWriter, r *http.Request) {
	// Parse scan run ID from path
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}

	runID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Subscribe to updates
	updates := h.scanner.Subscribe(runID)
	defer h.scanner.Unsubscribe(runID, updates)

	// Send initial state and check if already complete
	run, err := h.db.GetScanRun(runID)
	if err == nil {
		h.sendScanProgress(w, flusher, &types.ScanProgress{
			FilesScanned: run.FilesScanned,
			BytesScanned: run.BytesScanned,
			GroupsFound:  run.DuplicateGroups,
			WastedBytes:  run.WastedBytes,
			Status:       string(run.Status),
		})

		// If scan already completed, send complete event and wait briefly for client
		if run.Status != "running" {
			h.sendEvent(w, flusher, "complete", fmt.Sprintf(`{"status":"%s"}`, run.Status))
			h.waitForClientOrTimeout(r, 2*time.Second)
			return
		}
	}

	// Listen for updates
	for {
		select {
		case <-r.Context().Done():
			return
		case update, ok := <-updates:
			if !ok {
				// Channel closed (scan finished), send complete event
				h.sendEvent(w, flusher, "complete", `{"status":"completed"}`)
				h.waitForClientOrTimeout(r, 2*time.Second)
				return
			}
			h.sendScanProgress(w, flusher, update)
			if update.Status != "running" {
				h.sendEvent(w, flusher, "complete", fmt.Sprintf(`{"status":"%s"}`, update.Status))
				h.waitForClientOrTimeout(r, 2*time.Second)
				return
			}
		}
	}
}

// waitForClientOrTimeout waits for the client to disconnect or times out
func (h *Handler) waitForClientOrTimeout(r *http.Request, timeout time.Duration) {
	select {
	case <-r.Context().Done():
	case <-time.After(timeout):
	}
}

func (h *Handler) sendScanProgress(w http.ResponseWriter, flusher http.Flusher, progress *types.ScanProgress) {
	data := ScanProgressData{
		FilesScanned: progress.FilesScanned,
		BytesScanned: formatBytes(progress.BytesScanned),
		GroupsFound:  progress.GroupsFound,
		WastedBytes:  formatBytes(progress.WastedBytes),
		Status:       progress.Status,
		PhaseNum:     progress.PhaseNum,
		PhaseTotal:   progress.PhaseTotal,
		PhaseName:    progress.PhaseName,
		PhasePercent: progress.PhasePercent,
	}
	jsonData, _ := json.Marshal(data)
	h.sendEvent(w, flusher, "progress", string(jsonData))
}

func (h *Handler) sendEvent(w http.ResponseWriter, flusher http.Flusher, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	flusher.Flush()
}
