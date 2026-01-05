package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/lyall/kuron/internal/db"
)

// HistoryData holds data for the history template
type HistoryData struct {
	Title        string
	ActiveNav    string
	Runs         []*ScanRunHistoryView
	Actions      []*db.Action
	StatusFilter string
	Page         int
	HasMore      bool
	NextPage     int
}

// ScanRunHistoryView extends ScanRun with duration
type ScanRunHistoryView struct {
	*db.ScanRun
	Duration string
}

// History handles GET /history
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}

	statusFilter := r.URL.Query().Get("status")
	limit := 20
	offset := (page - 1) * limit

	// Get scan runs
	runs, err := h.db.ListScanRuns(limit+1, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hasMore := len(runs) > limit
	if hasMore {
		runs = runs[:limit]
	}

	// Convert to views with duration
	var runViews []*ScanRunHistoryView
	for _, run := range runs {
		view := &ScanRunHistoryView{ScanRun: run}
		if run.CompletedAt != nil {
			duration := run.CompletedAt.Sub(run.StartedAt)
			view.Duration = formatDuration(duration)
		} else if run.Status == db.ScanRunStatusRunning {
			view.Duration = "Running..."
		} else {
			view.Duration = "-"
		}
		runViews = append(runViews, view)
	}

	// Get recent actions
	actions, err := h.db.ListActions(10, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := HistoryData{
		Title:        "History",
		ActiveNav:    "history",
		Runs:         runViews,
		Actions:      actions,
		StatusFilter: statusFilter,
		Page:         page,
		HasMore:      hasMore,
		NextPage:     page + 1,
	}

	h.render(w, "history.html", data)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return strconv.Itoa(int(d.Seconds())) + "s"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return strconv.Itoa(m) + "m " + strconv.Itoa(s) + "s"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return strconv.Itoa(h) + "h " + strconv.Itoa(m) + "m"
}
