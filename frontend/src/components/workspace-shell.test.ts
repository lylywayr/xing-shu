import { describe, expect, it } from 'vitest'
import { mobileWorkspaceContract, responsiveWorkspaceContract } from './workspace-shell'

describe('New API inspired workspace contract', () => {
  it('uses an accessible mobile navigation drawer', () => {
    expect(mobileWorkspaceContract.navigation).toBe('drawer')
    expect(mobileWorkspaceContract.triggerLabel).toBe('打开导航')
    expect(mobileWorkspaceContract.closeLabel).toBe('关闭导航')
    expect(mobileWorkspaceContract.role).toBe('dialog')
  })

  it('keeps iOS form controls and touch targets usable', () => {
    expect(mobileWorkspaceContract.inputFontSize).toBe('16px')
    expect(mobileWorkspaceContract.touchTarget).toBe('40px')
    expect(mobileWorkspaceContract.contentBottomPadding).toBe('24px')
  })

  it('preserves desktop density while cardifying mobile lists', () => {
    expect(responsiveWorkspaceContract.desktopNavigation).toBe('grouped-sidebar')
    expect(responsiveWorkspaceContract.mobileList).toBe('card-list')
    expect(responsiveWorkspaceContract.contentMaxWidth).toBe('1600px')
    expect(responsiveWorkspaceContract.overflow).toBe('hidden-x')
  })
})
