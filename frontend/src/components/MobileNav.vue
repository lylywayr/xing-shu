<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { navGroups } from '../navigation'
import type { PageKey } from '../types'
import { focusableElements } from './dialog-a11y'

defineProps<{ page: PageKey }>()
const emit = defineEmits<{ navigate: [page: PageKey] }>()
const open = ref(false)
const drawer = ref<HTMLElement | null>(null)
const trigger = ref<HTMLElement | null>(null)
function show(origin?: HTMLElement) { trigger.value = origin || null; open.value = true }
function hide() { open.value = false }
function go(page: PageKey) { hide(); emit('navigate', page) }
function onKeydown(event: KeyboardEvent) {
  if (!open.value || !drawer.value) return
  if (event.key === 'Escape') { event.preventDefault(); hide(); return }
  if (event.key !== 'Tab') return
  const controls = focusableElements(drawer.value)
  if (!controls.length) return
  const index = controls.indexOf(document.activeElement as HTMLElement)
  if (event.shiftKey && (index <= 0 || index === -1)) { event.preventDefault(); controls[controls.length - 1].focus() }
  else if (!event.shiftKey && (index === controls.length - 1 || index === -1)) { event.preventDefault(); controls[0].focus() }
}
watch(open, async value => {
  document.body.classList.toggle('nav-drawer-open', value)
  if (value) { await nextTick(); focusableElements(drawer.value as HTMLElement)[0]?.focus(); document.addEventListener('keydown', onKeydown) }
  else { document.removeEventListener('keydown', onKeydown); await nextTick(); trigger.value?.focus() }
})
onBeforeUnmount(() => { document.removeEventListener('keydown', onKeydown); document.body.classList.remove('nav-drawer-open') })
defineExpose({ show, hide })
</script>

<template>
  <button class="mobile-menu-trigger" aria-label="打开导航" aria-controls="mobile-workspace-navigation" :aria-expanded="open" @click="show($event.currentTarget as HTMLElement)">☰</button>
  <div v-if="open" class="mobile-drawer-backdrop" @click.self="hide">
    <aside id="mobile-workspace-navigation" ref="drawer" class="mobile-drawer" role="dialog" aria-modal="true" aria-labelledby="mobile-navigation-title">
      <header class="mobile-drawer-head"><div class="brand"><div class="brand-mark">✦</div><div><strong id="mobile-navigation-title">星枢</strong><span>工作区导航</span></div></div><button class="icon-button" aria-label="关闭导航" @click="hide">×</button></header>
      <div class="mobile-drawer-scroll">
        <nav v-for="group in navGroups" :key="group.label" class="nav-group" :aria-label="group.label"><small>{{ group.label }}</small><button v-for="item in group.items" :key="item.key" class="nav-item" :class="{ active: page === item.key }" :aria-current="page === item.key ? 'page' : undefined" @click="go(item.key)"><em>{{ item.icon }}</em><span><b>{{ item.label }}</b><small>{{ item.description }}</small></span><i v-if="page === item.key"></i></button></nav>
      </div>
      <footer class="mobile-drawer-foot"><span class="pulse-dot"></span><span>星枢服务在线</span></footer>
    </aside>
  </div>
</template>
