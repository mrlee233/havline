package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/havline/havline/internal/metrics"
)

type MetricsHandler struct {
	store *metrics.Store
}

func NewMetricsHandler(store *metrics.Store) *MetricsHandler {
	return &MetricsHandler{store: store}
}

// History 返回指标历史：窗口内的序列已按桶取平均，前端拿到就能直接画。
// hours 最多 30 天（＝保留期），buckets 最多 metrics.MaxBuckets。
func (h *MetricsHandler) History(w http.ResponseWriter, r *http.Request) {
	hours := 24
	if raw := r.URL.Query().Get("hours"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 24*30 {
			hours = parsed
		}
	}
	buckets := 48
	if raw := r.URL.Query().Get("buckets"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			buckets = parsed
		}
	}
	if buckets > metrics.MaxBuckets {
		buckets = metrics.MaxBuckets
	}
	series, err := h.store.Query(r.Context(), time.Duration(hours)*time.Hour, buckets)
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取指标历史失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hours":   hours,
		"buckets": buckets,
		"series":  series,
	})
}
