export const mobileWorkspaceContract = {
  navigation: 'drawer',
  triggerLabel: '打开导航',
  closeLabel: '关闭导航',
  role: 'dialog',
  inputFontSize: '16px',
  touchTarget: '40px',
  contentBottomPadding: '24px',
} as const

export const responsiveWorkspaceContract = {
  desktopNavigation: 'grouped-sidebar',
  mobileList: 'card-list',
  contentMaxWidth: '1600px',
  overflow: 'hidden-x',
} as const
