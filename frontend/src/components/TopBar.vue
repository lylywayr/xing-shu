<script setup lang="ts">
defineProps<{ eyebrow: string; title: string; refreshing?: boolean }>()
const emit = defineEmits<{ menu: [origin: HTMLElement]; refresh: []; logout: [] }>()
</script>

<template>
  <header class="topbar">
    <div class="topbar-left">
      <button class="topbar-menu" aria-label="打开导航" @click="emit('menu', $event.currentTarget as HTMLElement)">☰</button>
      <div class="topbar-brand"><span class="brand-mark brand-mark-small">✦</span><strong>星枢</strong></div>
      <span class="topbar-divider"></span>
      <div class="topbar-title"><div class="eyebrow">控制台 / {{ eyebrow }}</div><h1>{{ title }}</h1></div>
    </div>
    <div class="top-actions">
      <span class="live-status"><i></i> 在线</span>
      <button class="icon-action" :disabled="refreshing" aria-label="刷新数据" @click="$emit('refresh')">{{ refreshing ? '…' : '↻' }}</button>
      <button class="avatar" aria-label="退出登录" @click="$emit('logout')">管</button>
    </div>
  </header>
</template>
