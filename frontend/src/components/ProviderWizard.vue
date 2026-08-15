<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import type { Provider } from '../types'
import { canValidateProvider, emptyProviderDraft, providerToDraft, validationSteps, type ProviderDraft, type ProviderValidation } from '../services/provider-registry'
import { post } from '../services/api'
import StatusBadge from './StatusBadge.vue'
import { focusableElements } from './dialog-a11y'

const props = defineProps<{ open: boolean; provider?: Provider | null; credentialReady: boolean }>()
const emit = defineEmits<{ close: []; saved: [message: string] }>()
const draft = ref<ProviderDraft>(emptyProviderDraft())
const validation = ref<ProviderValidation | null>(null)
const busy = ref(false)
const error = ref('')
const wizard = ref<HTMLElement | null>(null)
const editing = computed(() => !!props.provider)
const readOnly = computed(() => props.provider?.read_only === true)
const canValidate = computed(() => canValidateProvider(draft.value, editing.value) && (readOnly.value || props.credentialReady))
watch(() => [props.open, props.provider] as const, async ([open]) => {
  if (!open) return
  draft.value = props.provider ? providerToDraft(props.provider) : emptyProviderDraft(); validation.value = null; error.value = ''
  await nextTick(); focusableElements(wizard.value as HTMLElement)[0]?.focus(); document.addEventListener('keydown', onKeydown)
}, { immediate: true })
watch(() => props.open, open => { if (!open) document.removeEventListener('keydown', onKeydown) })
function onKeydown(event: KeyboardEvent) {
  if (!props.open || !wizard.value) return
  if (event.key === 'Escape') { event.preventDefault(); emit('close'); return }
  if (event.key !== 'Tab') return
  const controls = focusableElements(wizard.value); if (!controls.length) return
  const index = controls.indexOf(document.activeElement as HTMLElement)
  if (event.shiftKey && (index <= 0 || index === -1)) { event.preventDefault(); controls[controls.length - 1].focus() }
  else if (!event.shiftKey && (index === controls.length - 1 || index === -1)) { event.preventDefault(); controls[0].focus() }
}
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
function payload() { return { ...draft.value, api_key: draft.value.api_key || undefined } }
async function validate() {
  busy.value = true; error.value = ''; validation.value = null
  try { validation.value = await post<ProviderValidation>('/api/admin/provider-registry/validate', payload(), { timeoutMs: 55_000, retries: 0 }) }
  catch (e) { const candidate = e as { message?: string }; error.value = candidate.message || '验证失败' }
  finally { busy.value = false }
}
async function save() {
  if (!validation.value?.ok) return
  busy.value = true; error.value = ''
  try {
    const path = editing.value ? `/api/admin/provider-registry/update?id=${encodeURIComponent(draft.value.id)}` : '/api/admin/provider-registry/create'
    await post(path, payload(), { timeoutMs: 55_000, retries: 0 })
    emit('saved', editing.value ? 'Provider 已更新' : 'Provider 已接入')
  } catch (e) { error.value = (e as { message?: string }).message || '保存失败' }
  finally { busy.value = false }
}
</script>

<template>
  <div v-if="open" class="provider-wizard-layer" @click.self="emit('close')">
    <aside ref="wizard" class="provider-wizard" role="dialog" aria-modal="true" aria-labelledby="provider-wizard-title">
      <header class="provider-wizard-head"><div><span class="kicker">OPENAI COMPATIBLE</span><h2 id="provider-wizard-title">{{ readOnly ? '验证部署 Provider' : editing ? '编辑 Provider' : '添加 Provider' }}</h2><p>{{ readOnly ? '该 Provider 来自部署环境，只能验证，配置修改需更新环境变量。' : '填写配置后先完成四步真实验证，验证通过才会加密保存。' }}</p></div><button class="icon-button" aria-label="关闭 Provider 向导" @click="emit('close')">×</button></header>
      <div class="provider-wizard-body">
        <div v-if="!credentialReady && !readOnly" class="wizard-warning"><strong>加密主密钥未配置</strong><p>请先设置 XING_SHU_CREDENTIAL_KEY，动态 API Key 不允许明文落盘。</p></div>
        <div class="wizard-form">
          <label><span>Provider ID</span><input v-model.trim="draft.id" :disabled="editing" autocomplete="off" placeholder="例如 my-provider"/><small>仅小写字母、数字、点、下划线和连字符</small></label>
          <label><span>显示名称</span><input v-model.trim="draft.name" :disabled="readOnly" autocomplete="off" placeholder="例如 我的模型服务"/></label>
          <label class="wizard-wide"><span>OpenAI-compatible Base URL</span><input v-model.trim="draft.base_url" :disabled="readOnly" inputmode="url" autocomplete="url" placeholder="https://api.example.com 或 /v1"/><small>星枢会自动规范为 /v1，并验证 /models 与 /chat/completions。</small></label>
          <label v-if="!readOnly" class="wizard-wide"><span>API Key</span><input v-model="draft.api_key" type="password" autocomplete="new-password" :placeholder="editing ? '留空则保留现有密钥' : '输入 API Key'"/><small v-if="editing && provider?.api_key_mask">当前：{{ provider.api_key_mask }}</small></label>
          <div v-else class="wizard-wide readonly-secret"><span>API Key</span><strong>{{ provider?.api_key_mask || '由部署环境注入' }}</strong></div>
        </div>
        <section class="validation-panel"><div class="card-title"><span>连接验证</span><StatusBadge :label="validation?.ok ? '全部通过' : busy ? '验证中' : '等待验证'" :tone="validation?.ok ? 'green' : busy ? 'blue' : 'muted'"/></div><div class="validation-grid"><div v-for="step in validationSteps" :key="step.key" class="validation-step" :class="{ passed: validation?.[step.key]?.ok, failed: validation && !validation[step.key]?.ok }"><i>{{ validation?.[step.key]?.ok ? '✓' : validation ? '!' : '·' }}</i><span><strong>{{ step.label }}</strong><small>{{ validation?.[step.key]?.message || (validation?.[step.key]?.status ? `HTTP ${validation[step.key].status}` : '尚未执行') }}</small></span><em v-if="validation?.[step.key]?.latency_ms">{{ validation[step.key].latency_ms }}ms</em></div></div><p v-if="validation?.ok" class="validation-summary">发现 {{ validation.model_count }} 个模型，示例模型：{{ validation.sample_model }}</p></section>
        <p v-if="error" class="wizard-error">{{ error }}</p>
      </div>
      <footer class="provider-wizard-actions"><button class="button button-ghost" @click="emit('close')">取消</button><button class="button button-secondary" :disabled="!canValidate || busy" @click="validate">{{ busy ? '验证中…' : '验证连接' }}</button><button v-if="!readOnly" class="button button-primary" :disabled="!validation?.ok || busy" @click="save">{{ editing ? '保存更新' : '加密保存' }}</button></footer>
    </aside>
  </div>
</template>
