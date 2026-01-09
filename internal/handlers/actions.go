package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/lyallcooper/kuron/internal/db"
)

const actionGroupsPageSize = 50

// ActionDetailData holds data for the action detail template
type ActionDetailData struct {
	Title       string
	ActiveNav   string
	Action      *db.Action
	Run         *db.ScanRun // The scan run this action was from
	GroupsTable GroupsTableData
}

// ActionDetail handles GET /actions/{id}
func (h *Handler) ActionDetail(w http.ResponseWriter, r *http.Request) {
	// Parse action ID from URL: /actions/{id}
	path := strings.TrimPrefix(r.URL.Path, "/actions/")
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid action ID", http.StatusBadRequest)
		return
	}

	action, err := h.db.GetAction(id)
	if err != nil {
		http.Error(w, "Action not found", http.StatusNotFound)
		return
	}

	// Get associated scan run
	var run *db.ScanRun
	if action.ScanRunID > 0 {
		run, _ = h.db.GetScanRun(action.ScanRunID)
	}

	// Parse query params
	query := r.URL.Query()

	// Sorting
	sortBy := query.Get("sort")
	if sortBy == "" {
		sortBy = "wasted"
	}
	sortOrder := query.Get("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	// Pagination
	page := 1
	if p, err := strconv.Atoi(query.Get("page")); err == nil && p > 0 {
		page = p
	}

	totalCount := len(action.GroupIDs)
	totalPages := (totalCount + actionGroupsPageSize - 1) / actionGroupsPageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	// Fetch all groups and sort them
	var allGroups []*db.DuplicateGroup
	if totalCount > 0 {
		allGroups, _ = h.db.GetDuplicateGroupsByIDs(action.GroupIDs)
		sortGroups(allGroups, sortBy, sortOrder)
	}

	// Paginate after sorting
	var groups []*db.DuplicateGroup
	if len(allGroups) > 0 {
		start := (page - 1) * actionGroupsPageSize
		end := start + actionGroupsPageSize
		if end > len(allGroups) {
			end = len(allGroups)
		}
		groups = allGroups[start:end]
	}

	data := ActionDetailData{
		Title:     "Action Details",
		ActiveNav: "history",
		Action:    action,
		Run:       run,
		GroupsTable: GroupsTableData{
			Groups:      groups,
			Interactive: false,
			Page:        page,
			PageSize:    actionGroupsPageSize,
			TotalCount:  totalCount,
			TotalPages:  totalPages,
			SortBy:      sortBy,
			SortOrder:   sortOrder,
			BaseURL:     fmt.Sprintf("/actions/%d", id),
		},
	}

	h.render(w, "action_detail.html", data)
}

// sortGroups sorts groups in place by the specified column and order
func sortGroups(groups []*db.DuplicateGroup, sortBy, sortOrder string) {
	sort.Slice(groups, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "size":
			less = groups[i].FileSize < groups[j].FileSize
		case "count":
			less = groups[i].FileCount < groups[j].FileCount
		case "wasted":
			less = groups[i].WastedBytes < groups[j].WastedBytes
		default:
			less = groups[i].WastedBytes < groups[j].WastedBytes
		}
		if sortOrder == "desc" {
			return !less
		}
		return less
	})
}
