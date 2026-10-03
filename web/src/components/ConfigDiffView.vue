<template>
  <div class="config-diff">
    <div class="config-diff__head">
      <span class="config-diff__range">{{ fromLabel }} → {{ toLabel }}</span>
      <span class="config-diff__stats">
        <span class="config-diff__add">+{{ stats.added }}</span>
        <span class="config-diff__del">−{{ stats.removed }}</span>
      </span>
    </div>
    <div class="config-diff__body">
      <div
        v-for="(line, index) in lines"
        :key="index"
        class="config-diff__row"
        :class="`config-diff__row--${line.kind}`"
      >
        <span class="config-diff__sign">{{ sign(line.kind) }}</span>
        <span class="config-diff__text">{{ line.text || ' ' }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { diffLines, diffStats, type DiffLine } from '../utils/diff'

const props = withDefaults(
  defineProps<{
    fromText: string
    toText: string
    fromLabel?: string
    toLabel?: string
  }>(),
  { fromLabel: '当前', toLabel: '所选版本' },
)

const lines = computed<DiffLine[]>(() => diffLines(props.fromText, props.toText))
const stats = computed(() => diffStats(lines.value))

function sign(kind: DiffLine['kind']): string {
  if (kind === 'add') return '+'
  if (kind === 'del') return '−'
  return ' '
}
</script>

<style scoped>
.config-diff { border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); overflow: hidden; }
.config-diff__head {
  display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3);
  padding: 6px var(--havline-space-3); border-bottom: 1px solid var(--havline-border);
  background: var(--havline-bg-muted); font-size: 12px; color: var(--havline-text-secondary);
}
.config-diff__stats { display: flex; gap: 8px; font-family: var(--havline-mono); }
.config-diff__add { color: var(--havline-brand-text); }
.config-diff__del { color: var(--havline-error); }
.config-diff__body { max-height: 420px; overflow: auto; background: var(--havline-surface); }
.config-diff__row {
  display: flex; gap: 8px; padding: 1px 10px;
  font-family: var(--havline-mono); font-size: 12px; line-height: 1.6;
}
.config-diff__row--add { background: var(--havline-brand-soft); }
.config-diff__row--del { background: var(--havline-error-soft); }
.config-diff__sign { flex: none; width: 10px; color: var(--havline-text-muted); }
.config-diff__row--add .config-diff__sign { color: var(--havline-brand-text); }
.config-diff__row--del .config-diff__sign { color: var(--havline-error); }
.config-diff__text { white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
