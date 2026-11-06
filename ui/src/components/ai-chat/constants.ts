import { isMacPlatform } from '@/lib/platform'

export const AI_CHAT_TOGGLE_EVENT = 'kite:ai-chat-toggle'

export function getAIChatShortcutLabel() {
  return isMacPlatform() ? '⌘⇧A' : 'Ctrl+Shift+A'
}
