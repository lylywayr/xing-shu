<script setup lang="ts">
defineProps<{ label: string; tone?: 'blue' | 'green' | 'amber' | 'red' | 'muted' }>()
</script>
<template><span class="status-badge" :class="`tone-${tone || 'muted'}`"><i></i>{{ label }}</span></template>
