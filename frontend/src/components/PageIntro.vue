<script setup lang="ts">
defineProps<{ kicker: string; title: string; description: string; count?: string }>()
defineEmits<{ action: [] }>()
</script>
<template><div class="page-intro"><div><span class="kicker">{{ kicker }}</span><h2>{{ title }} <small v-if="count">{{ count }}</small></h2><p>{{ description }}</p></div><div class="intro-actions"><slot name="actions"></slot></div></div></template>
