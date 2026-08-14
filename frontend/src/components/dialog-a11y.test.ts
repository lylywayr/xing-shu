import { describe, expect, it } from 'vitest'
import { focusableElements, nextFocusIndex } from './dialog-a11y'

describe('dialog accessibility helpers', () => {
  it('finds only visible keyboard-focusable controls', () => {
    const root = document.createElement('div')
    root.innerHTML = '<button>one</button><button disabled>two</button><input /><a href="#">link</a><div tabindex="-1">skip</div>'
    expect(focusableElements(root).map(element => element.tagName)).toEqual(['BUTTON', 'INPUT', 'A'])
  })

  it('wraps tab focus in both directions', () => {
    expect(nextFocusIndex(0, 3, false)).toBe(1)
    expect(nextFocusIndex(2, 3, false)).toBe(0)
    expect(nextFocusIndex(0, 3, true)).toBe(2)
  })
})
