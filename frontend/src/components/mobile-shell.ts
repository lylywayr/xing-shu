export const mobileShellContract = {
  bottomInset: 'env(safe-area-inset-bottom)',
  navigationHeight: 'var(--mobile-nav-height)',
  contentPadding: 'var(--mobile-nav-height) + env(safe-area-inset-bottom)',
  moreLabel: '打开更多模块',
  closeLabel: '关闭更多模块',
  role: 'dialog',
} as const

export const mobileGlassContract = {
  frame: 'glass',
  navigation: 'floating-pill',
  topbar: 'compact-glass',
  contentInset: 'env(safe-area-inset-bottom)',
} as const
