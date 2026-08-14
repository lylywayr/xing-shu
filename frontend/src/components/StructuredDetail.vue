<script setup lang="ts">
const props = defineProps<{ value: Record<string, unknown> }>()
const entries = Object.entries(props.value || {})
function display(value: unknown) { if (value === null || value === undefined || value === '') return '暂无数据'; if (typeof value === 'object') return JSON.stringify(value, null, 2); return String(value) }
function label(key: string) { return key.replaceAll('_', ' ') }
</script>
<template><div class="structured-detail"><div v-for="[key, value] in entries" :key="key" class="detail-field"><span>{{ label(key) }}</span><pre v-if="typeof value === 'object'">{{ display(value) }}</pre><strong v-else>{{ display(value) }}</strong></div></div></template>
