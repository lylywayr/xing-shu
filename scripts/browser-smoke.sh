#!/bin/sh
set -eu
BROWSER="${MINIS_BROWSER_USE:-minis-browser-use}"
TAB_ID="${MINIS_BROWSER_TAB_ID:-0}"
ROUTER_URL="${ROUTER_BROWSER_URL:-http://127.0.0.1:12100}"
run() { "$BROWSER" --tab-id "$TAB_ID" --json "$1"; }
assert_page() {
  expression="$1"
  expected="$2"
  description="$3"
  result="$(run "{\"action\":\"execute_js\",\"script\":\"return JSON.stringify($expression)\"}")"
  normalized="$(printf '%s' "$result" | tr -d '\\')"
  printf '%s' "$normalized" | grep -Fq "$expected" || { echo "browser assertion failed: $description" >&2; echo "$result" >&2; exit 1; }
}
run "{\"action\":\"navigate\",\"url\":\"$ROUTER_URL/\"}" >/dev/null
run '{"action":"wait_for_dom_stable","timeout":10000}' >/dev/null
assert_page "({title:document.title,overflow:document.documentElement.scrollWidth>window.innerWidth})" '"overflow":false' "console loads without horizontal overflow"
assert_page "({models:document.body.innerText.includes('目录模型'),active:document.body.innerText.includes('活跃')})" '"models":true' "overview metrics are visible"
run '{"action":"execute_js","script":"const button=[...document.querySelectorAll(\"button\")].find(node=>node.textContent?.includes(\"资源\")); button?.click(); return JSON.stringify({clicked:Boolean(button),path:location.pathname})"}' >/dev/null
run '{"action":"wait_for_dom_stable","timeout":10000}' >/dev/null
run "{\"action\":\"navigate\",\"url\":\"$ROUTER_URL/?page=catalog&query=luna&status=active&p=2\"}" >/dev/null
run '{"action":"wait_for_dom_stable","timeout":10000}' >/dev/null
assert_page "({url:location.search,overflow:document.documentElement.scrollWidth>window.innerWidth})" '"url":"?page=catalog&query=luna&status=active&p=2"' "URL state survives navigation"
echo "browser smoke: navigation, mobile overflow, and URL persistence checks passed"
