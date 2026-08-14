<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { AsyncStatus } from '../services/async-state'

type State = Extract<AsyncStatus, 'loading' | 'timeout' | 'error' | 'empty'>
const props = withDefaults(defineProps<{ state: State; message?: string; detail?: string; action?: string }>(), { action: '重试' })
const emit = defineEmits<{ retry: [] }>()
const elapsed = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
const title = computed(() => props.state === 'loading' ? '正在读取数据' : props.state === 'timeout' ? '请求响应较慢' : props.state === 'empty' ? '暂无数据' : '数据加载失败')
const detailText = computed(() => props.detail || (props.state === 'timeout' ? '请求超过 3 秒仍未完成，可以重试或稍后查看。' : props.state === 'empty' ? '当前条件没有可展示的数据。' : props.state === 'error' ? '数据暂时不可用，请重试。' : ''))
onMounted(() => { if (props.state === 'loading') timer = setTimeout(() => { elapsed.value = true }, 3000) })
onBeforeUnmount(() => { if (timer) clearTimeout(timer) })
</script>
<template>
  <section class="load-state" :class="[`load-state-${state}`, { 'load-state-slow': elapsed }]" :aria-live="state === 'loading' ? 'polite' : 'assertive'">
    <template v-if="state === 'loading'"><div class="skeleton-stack" aria-hidden="true"><i></i><i></i><i></i></div><strong>{{ elapsed ? '仍在读取数据…' : title }}</strong></template>
    <template v-else><div class="load-state-icon" aria-hidden="true">{{ state === 'timeout' ? '◌' : state === 'empty' ? '∅' : '!' }}</div><strong>{{ message || title }}</strong><p>{{ detailText }}</p><button v-if="state !== 'empty'" class="button button-secondary" @click="emit('retry')">{{ action }}</button></template>
  </section>
</template>
