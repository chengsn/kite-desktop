export function isMacPlatform() {
  return typeof navigator !== 'undefined' && /mac/i.test(navigator.platform)
}

export function getModifierKeyLabel() {
  return isMacPlatform() ? '⌘' : 'Ctrl'
}
