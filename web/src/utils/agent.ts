/** Agent 版本展示：旧 Agent 未返回 agent_version 时降级为占位符，避免页面出现空白。 */
export function agentVersionText(version?: string): string {
  return version?.trim() || '—'
}
