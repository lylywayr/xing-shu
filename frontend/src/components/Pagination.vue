<script setup lang="ts">
defineProps<{ page: number; total: number; pageSize: number }>()
defineEmits<{ change: [page: number] }>()
</script>
<template><div v-if="total > pageSize" class="pagination"><span>{{ (page - 1) * pageSize + 1 }}–{{ Math.min(page * pageSize, total) }} / {{ total }}</span><div><button class="button button-ghost button-small" :disabled="page <= 1" @click="$emit('change', page - 1)">上一页</button><button class="button button-ghost button-small" :disabled="page * pageSize >= total" @click="$emit('change', page + 1)">下一页</button></div></div></template>
