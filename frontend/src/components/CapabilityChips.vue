<script setup lang="ts">
defineProps<{ model: { tools?: boolean; vision?: boolean; structured_output?: boolean; context_window?: number } }>()
</script>
<template><div class="capability-chips"><span v-if="model.tools" class="chip chip-green">Tools</span><span v-if="model.vision" class="chip chip-blue">Vision</span><span v-if="model.structured_output" class="chip chip-violet">JSON</span><span v-if="model.context_window" class="chip chip-muted">{{ Math.round(model.context_window / 1000) }}K ctx</span></div></template>
