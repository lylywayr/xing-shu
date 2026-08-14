<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { focusableElements } from './dialog-a11y'

const props = defineProps<{ open: boolean; title: string; detail: string; confirmLabel?: string; danger?: boolean; busy?: boolean }>()
const emit = defineEmits<{ confirm: []; cancel: [] }>()
const dialog = ref<HTMLElement | null>(null)
const lastFocused = ref<HTMLElement | null>(null)
const titleId = `confirm-title-${Math.random().toString(36).slice(2)}`

function focusDialog() {
  const controls = dialog.value ? focusableElements(dialog.value) : []
  controls[0]?.focus()
}
function onKeydown(event: KeyboardEvent) {
  if (!props.open || !dialog.value) return
  if (event.key === 'Escape' && !props.busy) {
    event.preventDefault()
    emit('cancel')
    return
  }
  if (event.key !== 'Tab') return
  const controls = focusableElements(dialog.value)
  if (!controls.length) return
  const current = document.activeElement as HTMLElement | null
  const index = controls.indexOf(current as HTMLElement)
  if (event.shiftKey && (index <= 0 || index === -1)) {
    event.preventDefault()
    controls[controls.length - 1].focus()
  } else if (!event.shiftKey && (index === controls.length - 1 || index === -1)) {
    event.preventDefault()
    controls[0].focus()
  }
}
watch(() => props.open, async open => {
  if (open) {
    lastFocused.value = document.activeElement as HTMLElement | null
    await nextTick()
    focusDialog()
    document.addEventListener('keydown', onKeydown)
  } else {
    document.removeEventListener('keydown', onKeydown)
    await nextTick()
    lastFocused.value?.focus()
    lastFocused.value = null
  }
})
onMounted(() => { if (props.open) document.addEventListener('keydown', onKeydown) })
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
</script>
<template>
  <div v-if="open" class="dialog-layer" role="presentation" @click.self="$emit('cancel')">
    <section ref="dialog" class="confirm-dialog" role="dialog" aria-modal="true" :aria-labelledby="titleId">
      <div class="dialog-icon" :class="{ danger }" aria-hidden="true">{{ danger ? '!' : '?' }}</div>
      <span class="kicker">CONFIRM ACTION</span>
      <h3 :id="titleId">{{ title }}</h3>
      <p>{{ detail }}</p>
      <div class="dialog-actions"><button class="button button-ghost" :disabled="busy" @click="$emit('cancel')">取消</button><button class="button" :class="danger ? 'button-danger' : 'button-primary'" :disabled="busy" @click="$emit('confirm')">{{ busy ? '执行中…' : (confirmLabel || '确认执行') }}</button></div>
    </section>
  </div>
</template>
