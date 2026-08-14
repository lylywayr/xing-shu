<script setup lang="ts">
defineProps<{ kicker: string; title: string; description: string; count?: string }>()
</script>
<template>
  <div class="page-intro">
    <div class="page-intro-copy"><span class="kicker">{{ kicker }}</span><div class="page-title-line"><h2>{{ title }}</h2><span v-if="count" class="page-count">{{ count }}</span></div><p>{{ description }}</p></div>
    <div class="intro-actions"><slot name="actions"></slot></div>
  </div>
</template>
