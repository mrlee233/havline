package frp

import (
	"context"
	"database/sql"
	"time"
)

// latestRouteHealthStates 取每条规则最新一条记录的状态，用于「同状态不重复写」判断
func (s *Store) latestRouteHealthStates(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.server_id, e.domain, e.state
		FROM route_health_events e
		JOIN (
			SELECT server_id, domain, MAX(id) AS id
			FROM route_health_events
			GROUP BY server_id, domain
		) m ON m.id = e.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := map[string]string{}
	for rows.Next() {
		var (
			serverID int64
			domain   string
			state    string
		)
		if err := rows.Scan(&serverID, &domain, &state); err != nil {
			return nil, err
		}
		states[routeHealthKey(serverID, domain)] = state
	}
	return states, rows.Err()
}

// routeHealthEventsSince 取窗口内的变更，外加每条规则在窗口开始前的最后一条
// ——否则「窗口开始时是什么状态」就无从得知了。
func (s *Store) routeHealthEventsSince(ctx context.Context, since time.Time) ([]RouteHealthEvent, error) {
	stamp := since.UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `
		SELECT server_id, domain, state, COALESCE(reason, ''), COALESCE(detail, ''), at
		FROM route_health_events WHERE at >= ?
		UNION ALL
		SELECT e.server_id, e.domain, e.state, COALESCE(e.reason, ''), COALESCE(e.detail, ''), e.at
		FROM route_health_events e
		JOIN (
			SELECT server_id, domain, MAX(id) AS id
			FROM route_health_events WHERE at < ?
			GROUP BY server_id, domain
		) m ON m.id = e.id`, stamp, stamp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]RouteHealthEvent, 0, 32)
	for rows.Next() {
		var event RouteHealthEvent
		if err := rows.Scan(&event.ServerID, &event.Domain, &event.State, &event.Reason, &event.Detail, &event.At); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// pruneRouteHealthEvents 清理 before 之前的记录，返回删除条数
func (s *Store) pruneRouteHealthEvents(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM route_health_events WHERE at < ?`, before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return affected, nil
}
