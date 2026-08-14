import { describe, expect, it } from 'vitest'
import { mobileGlassContract, mobileShellContract } from './mobile-shell'

describe('mobile shell contract', () => {
  it('requires safe-area variables and a stable navigation height', () => {
    expect(mobileShellContract.bottomInset).toContain('safe-area-inset-bottom')
    expect(mobileShellContract.navigationHeight).toBe('var(--mobile-nav-height)')
    expect(mobileShellContract.contentPadding).toContain('var(--mobile-nav-height)')
  })

  it('defines accessible controls for the more workspace', () => {
    expect(mobileShellContract.moreLabel).toBe('打开更多模块')
    expect(mobileShellContract.closeLabel).toBe('关闭更多模块')
    expect(mobileShellContract.role).toBe('dialog')
  })

  it('requires a floating glass mobile frame and compact top bar', () => {
    expect(mobileGlassContract.frame).toBe('glass')
    expect(mobileGlassContract.navigation).toBe('floating-pill')
    expect(mobileGlassContract.topbar).toBe('compact-glass')
    expect(mobileGlassContract.contentInset).toContain('safe-area-inset-bottom')
  })
})
