/* eslint-disable react-refresh/only-export-components */
import {
  createContext,
  ReactNode,
  useContext,
  useEffect,
  useState,
} from 'react'
import { useQueryClient } from '@tanstack/react-query'

import {
  checkDesktopUpdate,
  DESKTOP_LOCAL_RUNTIME,
  DESKTOP_WINDOW_STATE_CHANGE_EVENT,
  DesktopLiveWindowState,
  getDesktopWindowState,
  getDesktopStatus,
  getDesktopUpdateState,
} from '@/lib/desktop'

interface RuntimeContextType {
  isDesktop: boolean
  isImmersiveMacTitlebar: boolean
  /** True while the immersive macOS titlebar inset should be applied to the
   * main UI. It is dropped when the window is maximised or fullscreen so the
   * sidebar branding and content move up to the very top. */
  isTitlebarInsetActive: boolean
  isReady: boolean
}

const RuntimeContext = createContext<RuntimeContextType | undefined>(undefined)

export function RuntimeProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [state, setState] = useState<RuntimeContextType>({
    isDesktop: false,
    isImmersiveMacTitlebar: false,
    isTitlebarInsetActive: false,
    isReady: false,
  })

  useEffect(() => {
    let active = true
    let resizeTimer: ReturnType<typeof setTimeout> | undefined

    const applyWindowState = (windowState: DesktopLiveWindowState) => {
      if (!active) return
      setState((prev) => {
        const isTitlebarInsetActive =
          prev.isImmersiveMacTitlebar &&
          !windowState.maximised &&
          !windowState.fullscreen
        if (prev.isTitlebarInsetActive === isTitlebarInsetActive) {
          return prev
        }
        return { ...prev, isTitlebarInsetActive }
      })
    }

    const refreshWindowState = () => {
      if (!active) return
      void getDesktopWindowState().then(applyWindowState)
    }

    const handleWindowStateChange = (event: Event) => {
      const detail = (event as CustomEvent<DesktopLiveWindowState>).detail
      applyWindowState(
        detail && typeof detail.maximised === 'boolean'
          ? detail
          : { maximised: false, fullscreen: false }
      )
      // Re-query the authoritative state right after the notification: the
      // native query can lag behind the actual fullscreen/maximise transition.
      refreshWindowState()
    }

    const handleResize = () => {
      if (resizeTimer) {
        clearTimeout(resizeTimer)
      }
      // Fullscreen transitions fire a series of resize events; only query the
      // final one so the inset drops exactly when the transition settles.
      resizeTimer = setTimeout(refreshWindowState, 150)
    }

    window.addEventListener(
      DESKTOP_WINDOW_STATE_CHANGE_EVENT,
      handleWindowStateChange
    )
    window.addEventListener('resize', handleResize)
    window.addEventListener('focus', refreshWindowState)

    void getDesktopStatus()
      .then((status) => {
        if (!active) {
          return
        }

        const isDesktop =
          status.enabled && status.runtime === DESKTOP_LOCAL_RUNTIME
        setState({
          isDesktop,
          isImmersiveMacTitlebar:
            status.capabilities.immersiveMacTitlebar === true,
          isTitlebarInsetActive: false,
          isReady: true,
        })

        if (isDesktop) {
          refreshWindowState()
          void checkDesktopUpdate(false)
            .then(() => getDesktopUpdateState())
            .then((updateState) => {
              if (!active || !updateState) {
                return
              }
              queryClient.setQueryData(['desktop-update-state'], {
                ignoredVersion: updateState.ignoredVersion || '',
                lastCheck: updateState.lastCheck,
                download: updateState.download,
                readyToApply: updateState.readyToApply,
              })
            })
            .catch(() => undefined)
        }
      })
      .catch(() => {
        if (!active) {
          return
        }

        setState({
          isDesktop: false,
          isImmersiveMacTitlebar: false,
          isTitlebarInsetActive: false,
          isReady: true,
        })
      })

    return () => {
      active = false
      if (resizeTimer) {
        clearTimeout(resizeTimer)
      }
      window.removeEventListener(
        DESKTOP_WINDOW_STATE_CHANGE_EVENT,
        handleWindowStateChange
      )
      window.removeEventListener('resize', handleResize)
      window.removeEventListener('focus', refreshWindowState)
    }
  }, [])

  return (
    <RuntimeContext.Provider value={state}>{children}</RuntimeContext.Provider>
  )
}

export function useRuntime() {
  const context = useContext(RuntimeContext)
  if (!context) {
    throw new Error('useRuntime must be used within a RuntimeProvider')
  }
  return context
}
