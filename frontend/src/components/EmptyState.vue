<script setup lang="ts">
defineProps<{ title: string; detail: string; action?: string }>()
defineEmits<{ action: [] }>()
</script>
<template><div class="empty-state"><div class="empty-orb">∅</div><h3>{{ title }}</h3><p>{{ detail }}</p><button v-if="action" class="button button-secondary" @click="$emit('action')">{{ action }}</button></div></template>
