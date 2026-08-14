<script setup lang="ts">
import { navGroups } from '../navigation'
import type { PageKey } from '../types'

defineProps<{ page: PageKey; mobile?: boolean }>()
const emit = defineEmits<{ navigate: [page: PageKey] }>()
</script>

<template>
  <aside class="sidebar" :class="{ 'sidebar-mobile': mobile }">
    <div class="brand">
      <div class="brand-mark" aria-hidden="true">✦</div>
      <div><strong>星枢</strong><span>模型路由与治理</span></div>
    </div>
    <div class="sidebar-scroll">
      <nav v-for="group in navGroups" :key="group.label" class="nav-group" :aria-label="group.label">
        <small>{{ group.label }}</small>
        <button v-for="item in group.items" :key="item.key" class="nav-item" :class="{ active: page === item.key }" :aria-current="page === item.key ? 'page' : undefined" @click="emit('navigate', item.key)">
          <em aria-hidden="true">{{ item.icon }}</em>
          <span><b>{{ item.label }}</b><small v-if="mobile">{{ item.description }}</small></span>
          <i v-if="page === item.key" aria-hidden="true"></i>
        </button>
      </nav>
    </div>
    <div class="sidebar-foot"><span class="pulse-dot"></span><span>服务在线</span></div>
  </aside>
</template>
