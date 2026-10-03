package metrics

import (
	"testing"
	"time"
)

func TestBucketizeAveragesAndSkipsEmptyBuckets(t *testing.T) {
	from := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)

	// 12 个 5 分钟桶：第 0 桶两个点（取平均）、第 1 桶一个、第 11 桶一个
	at := []time.Time{
		from.Add(time.Minute),
		from.Add(2 * time.Minute),
		from.Add(6 * time.Minute),
		from.Add(59 * time.Minute),
	}
	values := []float64{10, 20, 30, 40}

	points := Bucketize(at, values, from, to, 12)
	if len(points) != 3 {
		t.Fatalf("空桶不该产出点（否则图上会出现凭空补 0 的谷底），实际 %d 个：%+v", len(points), points)
	}
	if points[0].Value != 15 {
		t.Fatalf("第 0 桶应为 (10+20)/2=15，实际 %v", points[0].Value)
	}
	if points[0].At != from.Format(time.RFC3339) {
		t.Fatalf("桶时间戳应为桶起点，实际 %q", points[0].At)
	}
	if points[1].Value != 30 || points[2].Value != 40 {
		t.Fatalf("后续桶取值不对：%+v", points)
	}
}

func TestBucketizeDropsOutOfWindowPoints(t *testing.T) {
	from := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)

	at := []time.Time{from.Add(-time.Minute), from.Add(30 * time.Minute), to.Add(time.Minute)}
	values := []float64{1, 2, 3}
	points := Bucketize(at, values, from, to, 12)
	if len(points) != 1 || points[0].Value != 2 {
		t.Fatalf("窗口外的点应被丢掉，实际 %+v", points)
	}

	if points := Bucketize(at[:1], values[:1], from, to, 12); len(points) != 0 {
		t.Fatalf("全在窗口外时应返回空，实际 %+v", points)
	}
}

func TestBucketizeRejectsDegenerateInput(t *testing.T) {
	from := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	values := []float64{1}

	if points := Bucketize([]time.Time{from}, values, from, from, 12); points != nil {
		t.Fatalf("零宽窗口应返回 nil，实际 %+v", points)
	}
	if points := Bucketize([]time.Time{from}, values, from, from.Add(time.Hour), 0); points != nil {
		t.Fatalf("桶数为 0 应返回 nil，实际 %+v", points)
	}
	if points := Bucketize(nil, nil, from, from.Add(time.Hour), 12); points != nil {
		t.Fatalf("没有样本应返回 nil，实际 %+v", points)
	}
}

func TestBucketizeKeepsPointsShorterThanValues(t *testing.T) {
	from := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	points := Bucketize([]time.Time{from.Add(time.Minute)}, nil, from, from.Add(time.Hour), 12)
	if len(points) != 0 {
		t.Fatalf("值与时刻数量不一致时应忽略多出的时刻，实际 %+v", points)
	}
}

func TestPercent(t *testing.T) {
	if got := Percent(0, 0); got != 0 {
		t.Fatalf("总量为 0 应返回 0，实际 %v", got)
	}
	if got := Percent(50, 200); got != 25 {
		t.Fatalf("期望 25，实际 %v", got)
	}
}
