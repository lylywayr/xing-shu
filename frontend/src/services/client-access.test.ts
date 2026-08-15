import { describe, expect, it } from 'vitest'
import { curlExample, pythonExample, javascriptExample, type ClientApiInfo } from './client-access'
const info:ClientApiInfo={base_url:'https://router.example/v1',models_endpoint:'https://router.example/v1/models',chat_endpoint:'https://router.example/v1/chat/completions',authentication:'Bearer client API key',mode:'required',scopes:['models:read','chat:write']}
describe('client access examples',()=>{
 it('uses the displayed endpoint and never embeds a real key',()=>{const text=curlExample(info);expect(text).toContain(info.chat_endpoint);expect(text).toContain('$XING_SHU_API_KEY');expect(text).not.toContain('xsk_')})
 it('uses OpenAI-compatible base URL',()=>{expect(pythonExample(info)).toContain(info.base_url);expect(javascriptExample(info)).toContain(info.base_url)})
})
