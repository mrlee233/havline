// 极简行级差异：Nginx 配置通常只有几十行，用 LCS 就够，不为此引入第三方 diff 依赖。

export interface DiffLine {
  kind: 'same' | 'add' | 'del'
  text: string
  oldLine?: number
  newLine?: number
}

// maxDiffCells 限制 LCS 表大小：超长文本（例如整份日志误粘进来）退化为「整体替换」，
// 避免 O(n*m) 的表把页面卡死。
const maxDiffCells = 250_000

export function diffLines(fromText: string, toText: string): DiffLine[] {
  const from = splitLines(fromText)
  const to = splitLines(toText)

  if (from.length * to.length > maxDiffCells) {
    return [
      ...from.map((text, index) => ({ kind: 'del' as const, text, oldLine: index + 1 })),
      ...to.map((text, index) => ({ kind: 'add' as const, text, newLine: index + 1 })),
    ]
  }

  // dp[i][j] = from[i..] 与 to[j..] 的最长公共子序列长度
  const dp: number[][] = Array.from({ length: from.length + 1 }, () => new Array<number>(to.length + 1).fill(0))
  for (let i = from.length - 1; i >= 0; i -= 1) {
    for (let j = to.length - 1; j >= 0; j -= 1) {
      dp[i][j] = from[i] === to[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])
    }
  }

  const out: DiffLine[] = []
  let i = 0
  let j = 0
  while (i < from.length && j < to.length) {
    if (from[i] === to[j]) {
      out.push({ kind: 'same', text: from[i], oldLine: i + 1, newLine: j + 1 })
      i += 1
      j += 1
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      out.push({ kind: 'del', text: from[i], oldLine: i + 1 })
      i += 1
    } else {
      out.push({ kind: 'add', text: to[j], newLine: j + 1 })
      j += 1
    }
  }
  while (i < from.length) {
    out.push({ kind: 'del', text: from[i], oldLine: i + 1 })
    i += 1
  }
  while (j < to.length) {
    out.push({ kind: 'add', text: to[j], newLine: j + 1 })
    j += 1
  }
  return out
}

export function diffStats(lines: DiffLine[]): { added: number; removed: number } {
  let added = 0
  let removed = 0
  for (const line of lines) {
    if (line.kind === 'add') added += 1
    else if (line.kind === 'del') removed += 1
  }
  return { added, removed }
}

function splitLines(text: string): string[] {
  if (text === '') return []
  return text.replace(/\r\n/g, '\n').replace(/\n+$/, '').split('\n')
}
