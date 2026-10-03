<template>
  <div class="empty-state">
    <div class="empty-state__icon">{{ icon }}</div>
    <h3 class="empty-state__title">{{ title }}</h3>
    <p class="empty-state__desc">{{ description }}</p>
    <div v-if="$slots.action" class="empty-state__action">
      <slot name="action" />
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string
    description: string
    icon?: string
  }>(),
  { icon: '○' },
)
</script>

<style scoped>
.empty-state {
  padding: var(--havline-space-6) var(--havline-space-5);
  text-align: center;
}

.empty-state__icon {
  width: 48px;
  height: 48px;
  margin: 0 auto var(--havline-space-4);
  border-radius: 50%;
  background: var(--havline-brand-soft);
  color: var(--havline-brand);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  font-weight: 600;
}

.empty-state__title {
  margin: 0 0 var(--havline-space-2);
  font-size: 16px;
  font-weight: 600;
  color: var(--havline-text);
}

.empty-state__desc {
  margin: 0 auto;
  max-width: 360px;
  font-size: 14px;
  line-height: 1.6;
  color: var(--havline-text-secondary);
}

.empty-state__action {
  margin-top: var(--havline-space-5);
}
</style>
