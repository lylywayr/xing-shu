<script setup lang="ts">
import { navGroups } from '../navigation'
import type { PageKey } from '../types'
defineProps<{ page: PageKey }>()
const emit = defineEmits<{ navigate: [page: PageKey] }>()
</script>
<template>
  <aside class="sidebar">
    <div class="brand"><div class="brand-mark">✦</div><div><strong>星枢</strong><span>模型路由 · 治理控制面</span></div></div>
    <nav v-for="group in navGroups" :key="group.label" class="nav-group"><small>{{ group.label }}</small><button v-for="item in group.items" :key="item.key" class="nav-item" :class="{ active: page === item.key }" @click="emit('navigate', item.key)"><em>{{ item.icon }}</em><span>{{ item.label }}</span><i v-if="page === item.key"></i></button></nav>
    <div class="sidebar-foot"><span class="pulse-dot"></span><span>生产控制面在线</span></div>
  </aside>
</template>
