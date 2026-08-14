#!/usr/bin/env node

const base = process.env.STARCORE_ADMIN_URL || 'http://127.0.0.1:12100'
const username = process.env.STARCORE_ADMIN_USER
const password = process.env.STARCORE_ADMIN_PASSWORD
const timeoutMs = Number(process.env.STARCORE_AUDIT_TIMEOUT_MS || 15000)
const reviewerProvider = process.env.STARCORE_AUDIT_REVIEWER_PROVIDER
const reviewerModel = process.env.STARCORE_AUDIT_REVIEWER_MODEL

const endpoints = [
  { path: '/health/ready', required: ['ready', 'models'] },
  { path: '/api/admin/status', required: ['public_models', 'stats'] },
  { path: '/api/admin/overview', required: ['health', 'consistency', 'providers', 'recent', 'alerts', 'generated_at'] },
  { path: '/api/admin/consistency', required: ['catalog', 'governance', 'ok'] },
  { path: '/api/admin/models', required: ['models'] },
  { path: '/api/admin/providers', required: ['providers'] },
  { path: '/api/admin/probe/batch/history', required: ['items'] },
  { path: '/api/admin/governance/rules/audit', required: ['items'] },
  { path: '/api/admin/reviewer', required: ['model', 'provider'] },
  { path: '/api/admin/reviewer/test', method: 'POST', body: { provider: reviewerProvider, model: reviewerModel }, required: ['network', 'http', 'json', 'structured_output'] },
  { path: '/api/admin/reviews/stats', required: ['stats'] },
  { path: '/api/admin/reviewer/candidates', required: ['items', 'eligible', 'probeable', 'excluded'] },
  { path: '/api/admin/reviews?limit=20&offset=0', required: ['items', 'total', 'limit', 'offset'] },
  { path: '/api/admin/reviews/batches', required: ['items', 'total', 'limit', 'offset'] },
  { path: '/api/admin/routing/explain', required: ['items'] },
  { path: '/api/admin/governance', required: ['items'] },
  { path: '/api/admin/knowledge', required: ['items', 'stats', 'total'] },
  { path: '/api/admin/shadow', required: ['items'] },
  { path: '/api/admin/shadow/batches', required: ['items', 'total', 'limit', 'offset'] },
  { path: '/api/admin/recent?limit=20&model=', required: ['requests', 'total', 'limit', 'offset'] },
  { path: '/api/admin/alerts', required: ['items', 'total', 'limit', 'offset'] },
  { path: '/api/admin/diagnostics', required: ['items', 'data_dir', 'generated_at'] },
  { path: '/api/admin/watchdog', required: ['healthy', 'consecutive_failures', 'max_failures', 'detail'] },
  { path: '/api/admin/accounts', required: ['accounts'] },
  { path: '/api/admin/quota', required: ['items', 'available', 'message'] },
  { path: '/api/admin/quota/history', required: ['items', 'available'] },
  { path: '/api/admin/quota/status', required: ['configured', 'available', 'status', 'sources', 'items'] },
  { path: '/api/admin/quota/refresh', method: 'POST', body: {}, required: ['ok', 'results', 'items'] },
  { path: '/api/admin/snapshots', required: ['files'] },
]

function fail(message) { throw new Error(message) }
async function fetchJson(path, options = {}) {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)
  try {
    const response = await fetch(`${base}${path}`, { ...options, signal: controller.signal })
    const text = await response.text()
    let body = null
    if (text) {
      try { body = JSON.parse(text) } catch {
        if (response.ok) fail(`${path}: response is not JSON`)
      }
    }
    return { response, body, bytes: Buffer.byteLength(text) }
  } finally { clearTimeout(timer) }
}

async function main() {
  const unauth = await fetchJson('/api/admin/models')
  if (unauth.response.status !== 401) fail(`auth contract: expected 401, got ${unauth.response.status}`)
  if (!username || !password) fail('set STARCORE_ADMIN_USER and STARCORE_ADMIN_PASSWORD to run authenticated checks')
  const login = await fetchJson('/api/admin/login', { method: 'POST', headers: { 'content-type': 'application/x-www-form-urlencoded' }, body: new URLSearchParams({ username, password }).toString() })
  if (!login.response.ok) fail(`login contract: HTTP ${login.response.status}`)
  const cookieValues = login.response.headers.getSetCookie?.() || []
  const cookie = cookieValues[0] || login.response.headers.get('set-cookie')
  if (!cookie) fail('login contract: missing session cookie')
  const cookieHeader = (Array.isArray(cookie) ? cookie[0] : cookie).split(';', 1)[0]
  let warnings = 0
  for (const endpoint of endpoints) {
    if (endpoint.path === '/api/admin/reviewer/test' && (!reviewerProvider || !reviewerModel)) {
      warnings += 1
      console.warn('WARN reviewer: set STARCORE_AUDIT_REVIEWER_PROVIDER and STARCORE_AUDIT_REVIEWER_MODEL to test an active upstream model')
      continue
    }
    const options = { headers: { cookie: cookieHeader } }
    if (endpoint.method) { options.method = endpoint.method; options.headers['content-type'] = 'application/json'; options.body = JSON.stringify(endpoint.body) }
    const result = await fetchJson(endpoint.path, options)
    if (!result.response.ok) fail(`${endpoint.path}: HTTP ${result.response.status}`)
    if (!result.body || typeof result.body !== 'object') fail(`${endpoint.path}: body is not an object`)
    for (const key of endpoint.required) if (!(key in result.body)) fail(`${endpoint.path}: missing field ${key}`)
    if (result.bytes > 2_000_000) fail(`${endpoint.path}: response too large (${result.bytes} bytes)`)
    if (endpoint.path === '/api/admin/quota' && result.body.items === null) { warnings += 1; console.warn('WARN quota: provider has not supplied quota data; UI must show an explicit unavailable state') }
    if (endpoint.path === '/api/admin/accounts' && result.body.configured === false) { warnings += 1; console.warn('WARN accounts: account management is not configured') }
  }
  console.log(`contract audit passed: ${endpoints.length} authenticated endpoints; warnings=${warnings}`)
}

main().catch(error => { console.error(`contract audit failed: ${error.message}`); process.exitCode = 1 })
