<template>
  <div class="havline-card" :class="{ 'havline-card--flush': flush }">
    <div v-if="title || $slots.title || $slots.header" class="havline-card__header">
      <div>
        <h3 v-if="title" class="havline-card__title">{{ title }}</h3>
        <div v-else-if="$slots.title" class="havline-card__title havline-card__title-row">
          <slot name="title" />
        </div>
        <p v-if="subtitle" class="havline-card__subtitle">{{ subtitle }}</p>
      </div>
      <div v-if="$slots.header" class="havline-card__header-extra">
        <slot name="header" />
      </div>
    </div>
    <div class="havline-card__body">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title?: string
  subtitle?: string
  flush?: boolean
}>()
</script>

<style scoped>
.havline-card {
  background: var(--havline-surface);
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius);
  box-shadow: var(--havline-shadow);
  overflow: hidden;
}

.havline-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--havline-space-4);
  padding: var(--havline-space-5) var(--havline-space-5) 0;
}

.havline-card__title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--havline-text);
}

.havline-card__title-row {
  display: flex;
  align-items: center;
  gap: var(--havline-space-2);
}

.havline-card__subtitle {
  margin: var(--havline-space-1) 0 0;
  font-size: 13px;
  color: var(--havline-text-secondary);
}

.havline-card__body {
  padding: var(--havline-space-5);
}

.havline-card--flush .havline-card__body {
  padding-top: 0;
}
</style>
