ALTER TABLE frp_runtime_status ADD COLUMN desired_state TEXT NOT NULL DEFAULT 'stopped';
ALTER TABLE frp_runtime_status ADD COLUMN process_state TEXT NOT NULL DEFAULT 'stopped';
ALTER TABLE frp_runtime_status ADD COLUMN connection_state TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE frp_runtime_status ADD COLUMN state_source TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE frp_runtime_status ADD COLUMN process_pid INTEGER;
ALTER TABLE frp_runtime_status ADD COLUMN exit_code INTEGER;
ALTER TABLE frp_runtime_status ADD COLUMN last_connected_at TEXT;
ALTER TABLE frp_runtime_status ADD COLUMN last_disconnected_at TEXT;

UPDATE frp_runtime_status
SET desired_state = CASE WHEN status IN ('running', 'starting') THEN 'running' ELSE 'stopped' END,
    process_state = CASE WHEN status IN ('running', 'starting') THEN 'unknown' ELSE 'stopped' END,
    connection_state = CASE WHEN status = 'running' THEN 'unknown' ELSE 'unknown' END,
    state_source = 'migration'
WHERE desired_state = 'stopped' AND status IN ('running', 'starting');

CREATE INDEX IF NOT EXISTS idx_frp_runtime_process_state
ON frp_runtime_status(process_state, connection_state);
