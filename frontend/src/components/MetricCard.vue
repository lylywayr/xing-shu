<script setup lang="ts">
defineProps<{ label: string; value: string | number; note?: string; tone?: string; icon?: string }>()
</script>
<template><article class="metric-card" :class="tone ? `metric-${tone}` : ''"><div class="metric-head"><span>{{ label }}</span><b>{{ icon || '·' }}</b></div><strong>{{ value }}</strong><small v-if="note">{{ note }}</small></article></template>
