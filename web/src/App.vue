<template>
  <n-config-provider :theme="naiveTheme" :theme-overrides="themeOverrides">
    <n-message-provider>
      <n-dialog-provider>
        <router-view />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { NConfigProvider, NDialogProvider, NMessageProvider } from 'naive-ui'
import { useTheme } from './composables/useTheme'
import './styles/variables.css'

const { naiveTheme, themeOverrides } = useTheme()
</script>

<style>
* {
  box-sizing: border-box;
}

body {
  margin: 0;
  font-family: var(--havline-font);
  background: var(--havline-bg);
  color: var(--havline-text);
}

.mono {
  font-family: var(--havline-mono);
}

.text-secondary {
  color: var(--havline-text-secondary);
}

.text-muted {
  color: var(--havline-text-muted);
}

.page-shell,
.page-content {
  width: 100%;
}

.page-narrow {
  width: 100%;
  max-width: var(--havline-form-max);
  margin-inline: auto;
}

.form-narrow {
  max-width: var(--havline-form-max);
}

.toolbar {
  display: flex;
  align-items: center;
  gap: var(--havline-space-3);
  flex-wrap: wrap;
  margin-bottom: var(--havline-space-4);
  padding-top: var(--havline-space-4);
}

.n-data-table .n-data-table-th {
  background: var(--havline-bg-muted) !important;
  font-weight: 500;
  font-size: 13px;
  color: var(--havline-text-secondary);
}
</style>
