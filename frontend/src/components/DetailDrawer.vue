<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { focusableElements } from './dialog-a11y'

const props = defineProps<{ open: boolean; title: string; subtitle?: string }>()
const emit = defineEmits<{ close: [] }>()
const drawer = ref<HTMLElement | null>(null)
const lastFocused = ref<HTMLElement | null>(null)
const titleId = `drawer-title-${Math.random().toString(36).slice(2)}`

function focusDrawer() { const controls = drawer.value ? focusableElements(drawer.value) : []; controls[0]?.focus() }
function onKeydown(event: KeyboardEvent) {
  if (!props.open || !drawer.value) return
  if (event.key === 'Escape') { event.preventDefault(); emit('close'); return }
  if (event.key !== 'Tab') return
  const controls = focusableElements(drawer.value)
  if (!controls.length) return
  const index = controls.indexOf(document.activeElement as HTMLElement)
  if (event.shiftKey && (index <= 0 || index === -1)) { event.preventDefault(); controls[controls.length - 1].focus() }
  else if (!event.shiftKey && (index === controls.length - 1 || index === -1)) { event.preventDefault(); controls[0].focus() }
}
watch(() => props.open, async open => {
  if (open) { lastFocused.value = document.activeElement as HTMLElement | null; await nextTick(); focusDrawer(); document.addEventListener('keydown', onKeydown) }
  else { document.removeEventListener('keydown', onKeydown); await nextTick(); lastFocused.value?.focus(); lastFocused.value = null }
})
onMounted(() => { if (props.open) document.addEventListener('keydown', onKeydown) })
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
</script>
<template><div v-if="open" class="drawer-layer" role="presentation" @click.self="$emit('close')"><aside ref="drawer" class="detail-drawer" role="dialog" aria-modal="true" :aria-labelledby="titleId"><div class="drawer-head"><div><span class="kicker">DETAIL VIEW</span><h3 :id="titleId">{{ title }}</h3><small v-if="subtitle">{{ subtitle }}</small></div><button class="icon-button" aria-label="关闭详情" @click="$emit('close')">×</button></div><div class="drawer-body"><slot></slot></div></aside></div></template>
