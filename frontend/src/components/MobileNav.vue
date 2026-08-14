<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { mobileMore, mobilePrimary } from '../navigation'
import type { PageKey } from '../types'
import { focusableElements } from './dialog-a11y'

defineProps<{ page: PageKey }>()
const emit = defineEmits<{ navigate: [page: PageKey] }>()
const open = ref(false)
const sheet = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
function go(page: PageKey) { open.value = false; emit('navigate', page) }
function onKeydown(event: KeyboardEvent) {
  if (!open.value || !sheet.value) return
  if (event.key === 'Escape') { event.preventDefault(); open.value = false; return }
  if (event.key !== 'Tab') return
  const controls = focusableElements(sheet.value)
  const index = controls.indexOf(document.activeElement as HTMLElement)
  if (!controls.length) return
  if (event.shiftKey && (index <= 0 || index === -1)) { event.preventDefault(); controls[controls.length - 1].focus() }
  else if (!event.shiftKey && (index === controls.length - 1 || index === -1)) { event.preventDefault(); controls[0].focus() }
}
watch(open, async value => {
  if (value) { await nextTick(); focusableElements(sheet.value as HTMLElement)[0]?.focus(); document.addEventListener('keydown', onKeydown) }
  else { document.removeEventListener('keydown', onKeydown); await nextTick(); trigger.value?.focus() }
})
</script>
<template>
  <nav class="mobile-navigation" aria-label="主导航">
    <button v-for="item in mobilePrimary" :key="item.key" class="mobile-nav-item" :class="{ active: page === item.key }" :aria-current="page === item.key ? 'page' : undefined" @click="go(item.key)"><em aria-hidden="true">{{ item.icon }}</em><span>{{ item.label }}</span></button>
    <button ref="trigger" class="mobile-nav-item" :class="{ active: open || mobileMore.some(item => item.key === page) }" :aria-expanded="open" aria-controls="more-workspace" aria-label="打开更多模块" @click="open = !open"><em aria-hidden="true">•••</em><span>更多</span></button>
  </nav>
  <div v-if="open" class="more-sheet-backdrop" @click.self="open = false"><section id="more-workspace" ref="sheet" class="more-sheet" role="dialog" aria-modal="true" aria-labelledby="more-title"><div class="sheet-handle" aria-hidden="true"></div><div class="sheet-title"><div><span class="kicker">星枢控制面</span><h3 id="more-title">更多模块</h3></div><button class="icon-button" aria-label="关闭更多模块" @click="open = false">×</button></div><div class="more-grid"><button v-for="item in mobileMore" :key="item.key" class="more-item" :class="{ active: page === item.key }" :aria-current="page === item.key ? 'page' : undefined" @click="go(item.key)"><em aria-hidden="true">{{ item.icon }}</em><span>{{ item.label }}</span><i v-if="page === item.key">当前</i></button></div></section></div>
</template>
