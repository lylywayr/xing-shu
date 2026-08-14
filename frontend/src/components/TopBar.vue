<script setup lang="ts">
defineProps<{ eyebrow: string; title: string; refreshing?: boolean }>()
defineEmits<{ refresh: []; logout: [] }>()
</script>
<template><header class="topbar"><div><div class="eyebrow">星枢 <span>/</span> {{ eyebrow }}</div><h1>{{ title }}</h1></div><div class="top-actions"><span class="live-status"><i></i> LIVE</span><button class="button button-ghost" :disabled="refreshing" @click="$emit('refresh')">{{ refreshing ? '同步中…' : '刷新数据' }} <span>↻</span></button><button class="button button-ghost button-small logout-button" @click="$emit('logout')">退出</button><div class="avatar">LY</div></div></header></template>
