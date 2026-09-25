// Package metrics 保存指标历史（纵向 (scope, metric, at) 点表）并按窗口查询。
//
// 采样跟随巡检轮次（2 分钟），行量极小，所以不建降采样表：查询时按桶取平均即可，
// 这样只有一套数据、没有「原始数据与聚合数据不一致」这类问题。
package metrics

import (
	"context"
	"database/sql"
	"sort"
	"time"
)

const (
	// ScopeHost 本机资源；server:<id> / route:<serverID>:<domain> 留给后续序列
	ScopeHost = "host"

	// 指标名：与前端展示一一对应，改动即改动契约
	MetricCPU       = "cpu_percent"
	MetricMemUsed   = "mem_used_bytes"
	MetricMemTotal  = "mem_total_bytes"
	MetricDiskUsed  = "disk_used_bytes"
	MetricDiskTotal = "disk_total_bytes"

	// 流量序列按规则区分：scope 形如 route:<serverID>:<domain>
	MetricTrafficIn  = "traffic_in_rate"
	MetricTrafficOut = "traffic_out_rate"

	// MaxBuckets 单次查询最多返回的桶数（前端最多画这么多点）
	MaxBuckets = 720
	// Retention 保留期
	Retention = 30 * 24 * time.Hour
)

// HostMetrics 是本机序列的固定集合（顺序即前端展示顺序）
var HostMetrics = []string{MetricCPU, MetricMemUsed, MetricMemTotal, MetricDiskUsed, MetricDiskTotal}

// Sample 一条待写入的样本
type Sample struct {
	Scope  string
	Metric string
	Value  float64
}

// Point 查询结果里的一个点（bucket 为平均值；At 是该桶的起始时刻）
type Point struct {
	At    string  `json:"at"`
	Value float64 `json:"value"`
}

// Series 一条序列
type Series struct {
	Scope  string  `json:"scope"`
	Metric string  `json:"metric"`
	Points []Point `json:"points"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Record 批量写入一批样本（同一事务，避免每点一个来回）
func (s *Store) Record(ctx context.Context, samples []Sample) error {
	if len(samples) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO metrics_samples(scope, metric, value, at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, sample := range samples {
		if sample.Scope == "" || sample.Metric == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, sample.Scope, sample.Metric, sample.Value, now); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Prune 删除超过保留期的样本
func (s *Store) Prune(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM metrics_samples WHERE at < ?`, time.Now().UTC().Add(-Retention).Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return affected, nil
}

// Query 取窗口内的序列并按 buckets 个等宽桶聚合（每桶取平均）。
// buckets <= 0 时按原始点返回；都会按 MaxBuckets 截断。
func (s *Store) Query(ctx context.Context, window time.Duration, buckets int) ([]Series, error) {
	if window <= 0 {
		window = 24 * time.Hour
	}
	if buckets <= 0 {
		buckets = 48
	}
	if buckets > MaxBuckets {
		buckets = MaxBuckets
	}
	to := time.Now().UTC()
	from := to.Add(-window)

	rows, err := s.db.QueryContext(ctx, `
		SELECT scope, metric, at, value FROM metrics_samples
		WHERE at >= ? ORDER BY scope, metric, at`, from.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type raw struct {
		scope, metric string
		at            time.Time
		value         float64
	}
	grouped := map[string][]raw{}
	order := make([]string, 0, 8)
	for rows.Next() {
		var (
			scope, metric, at string
			value             float64
		)
		if err := rows.Scan(&scope, &metric, &at, &value); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339, at)
		if err != nil {
			continue
		}
		key := scope + "\x00" + metric
		if _, ok := grouped[key]; !ok {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], raw{scope: scope, metric: metric, at: parsed, value: value})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	series := make([]Series, 0, len(order))
	for _, key := range order {
		points := grouped[key]
		at := make([]time.Time, 0, len(points))
		values := make([]float64, 0, len(points))
		for _, item := range points {
			at = append(at, item.at)
			values = append(values, item.value)
		}
		if buckets <= 0 {
			buckets = 48
		}
		series = append(series, Series{
			Scope:  points[0].scope,
			Metric: points[0].metric,
			Points: Bucketize(at, values, from, to, buckets),
		})
	}
	// 固定顺序：本机序列按 HostMetrics 的顺序排，便于前端稳定取用
	sort.SliceStable(series, func(i, j int) bool {
		return metricOrder(series[i].Metric) < metricOrder(series[j].Metric)
	})
	return series, nil
}

// Bucketize 把 (时刻, 值) 序列按 [from, to] 等宽分成 buckets 个桶并取平均。
// 空桶不产出点（前端画线时自动跨过，不会出现「凭空补 0」的假谷底）。
func Bucketize(at []time.Time, values []float64, from, to time.Time, buckets int) []Point {
	if buckets <= 0 || to.Before(from) || len(at) == 0 {
		return nil
	}
	span := to.Sub(from)
	if span <= 0 {
		return nil
	}
	width := span / time.Duration(buckets)
	if width <= 0 {
		return nil
	}

	sums := make([]float64, buckets)
	counts := make([]int, buckets)
	for i, moment := range at {
		if i >= len(values) {
			break
		}
		if moment.Before(from) || moment.After(to) {
			continue
		}
		index := int(moment.Sub(from) / width)
		if index >= buckets {
			index = buckets - 1
		}
		if index < 0 {
			continue
		}
		sums[index] += values[i]
		counts[index]++
	}

	points := make([]Point, 0, buckets)
	for i := 0; i < buckets; i++ {
		if counts[i] == 0 {
			continue
		}
		points = append(points, Point{
			At:    from.Add(time.Duration(i) * width).UTC().Format(time.RFC3339),
			Value: sums[i] / float64(counts[i]),
		})
	}
	return points
}

func metricOrder(metric string) int {
	for index, name := range HostMetrics {
		if name == metric {
			return index
		}
	}
	return len(HostMetrics) + 1
}

// Percent 由「已用 / 总量」算百分比：总量为 0 时返回 0（而不是 NaN）
func Percent(used, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return used / total * 100
}
