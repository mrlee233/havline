<template>
  <span class="status-badge" :class="[kind, { pill }]">
    <span class="dot" />
    <span class="label">{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { statusKind, statusLabel, type StatusKind } from '../utils/status'

const props = withDefaults(
  defineProps<{
    value?: string
    text?: string
    kind?: StatusKind
    pill?: boolean
  }>(),
  { pill: true },
)

const kind = computed(() => props.kind ?? statusKind(props.value))
const label = computed(() => props.text ?? statusLabel(props.value))
</script>

<style scoped>
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 500;
  line-height: 1;
}

.status-badge.pill {
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--havline-bg);
}

.status-badge.pill.success {
  background: rgba(16, 185, 129, 0.12);
  color: var(--havline-success);
}

.status-badge.pill.warning {
  background: rgba(245, 158, 11, 0.12);
  color: var(--havline-warning);
}

.status-badge.pill.error {
  background: rgba(239, 68, 68, 0.12);
  color: var(--havline-error);
}

.status-badge.pill.disabled,
.status-badge.pill.unknown {
  background: var(--havline-bg-muted);
  color: var(--havline-text-secondary);
}

.dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}

.success .dot { background: var(--havline-success); }
.warning .dot { background: var(--havline-warning); }
.error .dot { background: var(--havline-error); }
.disabled .dot { background: var(--havline-disabled); }
.unknown .dot { background: var(--havline-text-muted); }

.success .label { color: inherit; }
.warning .label { color: inherit; }
.error .label { color: inherit; }
.disabled .label { color: inherit; }
.unknown .label { color: inherit; }

.status-badge:not(.pill) .success .label { color: var(--havline-success); }
.status-badge:not(.pill) .warning .label { color: var(--havline-warning); }
.status-badge:not(.pill) .error .label { color: var(--havline-error); }
.status-badge:not(.pill) .disabled .label,
.status-badge:not(.pill) .unknown .label { color: var(--havline-text-secondary); }
</style>
