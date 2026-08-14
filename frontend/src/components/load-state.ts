export const loadStateContract = {
  states: ['loading', 'timeout', 'error', 'empty'] as const,
  timeoutAfterMs: 3000,
  retryLabel: '重试',
} as const
