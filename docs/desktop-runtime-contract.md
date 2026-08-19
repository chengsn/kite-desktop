# Desktop Runtime Contract

## Scope

This document defines the runtime contract between the embedded desktop host and the React UI.

It applies only when:

- `APP_RUNTIME=desktop-local`
- `/api/desktop/status.enabled=true`

## Runtime Identity

Desktop mode is represented as a dedicated runtime, not as anonymous web mode.

Runtime response shape:

```json
{
  "enabled": true,
  "runtime": "desktop-local",
  "capabilities": {
    "nativeFileDialog": true,
    "nativeSaveDialog": true,
    "tray": true,
    "menu": true,
    "singleInstance": true,
    "immersiveMacTitlebar": true
  }
}
```

`immersiveMacTitlebar` is only `true` on macOS. It means every window (main
window, AI chat sidecar, child windows) uses an immersive title bar: the
native traffic lights float over the webview content, the native title bar
strip is gone, and the top edge (32px) of the main / child windows is a native
drag region for moving the window. The AI chat sidecar intentionally has no
native drag strip so its header row stays interactive; it remains draggable
through the traffic lights.

## Immersive macOS Titlebar

When `capabilities.immersiveMacTitlebar` is `true`, the UI must keep the top
of the window clear of the traffic lights:

- the sidebar branding shifts down (`--mac-titlebar-inset`, 2rem)
- the content header / scrollable area starts below the same inset
- the standalone AI chat header shifts its title right instead of applying
  the top inset, because the sidecar has no drag strip
- the standalone AI chat header starts a native drag (`wails:drag` through
  the Wails webview bridge) when pressed on non-interactive areas, so the
  sidecar stays draggable by its top even without a native drag strip

While the window is maximised or fullscreen the inset is dropped: the sidebar
branding and content area move up to the very top. The UI tracks this through
`GET /api/desktop/window/state` and the `kite:window-state-change` event
(consumed in `ui/src/contexts/runtime-context.tsx`), then re-queries the
authoritative state and also refreshes on webview `resize` / `focus` so the
inset settles reliably even across fullscreen transitions; page components
should not add ad-hoc padding based on browser/platform sniffing.

## Auth Contract

Desktop mode does not synthesize a frontend-only user.

The UI must always read the current user from:

- `GET /api/auth/user`

The backend injects the local desktop user when `desktop-local` runtime is active.

## Desktop APIs

### `GET /api/desktop/status`

Returns current runtime and desktop capabilities.

### `GET /api/desktop/app-info`

Returns app metadata and resolved local directories.

Response shape:

```json
{
  "name": "Kite",
  "runtime": "desktop-local",
  "version": "v0.0.0",
  "buildDate": "2026-04-04T00:00:00Z",
  "commitId": "abcdef0",
  "paths": {
    "configDir": "...",
    "logsDir": "...",
    "cacheDir": "...",
    "tempDir": "..."
  }
}
```

### `POST /api/desktop/open-url`

Opens:

- same-origin routes in a desktop child window
- external links in the system browser

### `POST /api/desktop/open-file`

Opens the native file picker and can optionally return file content.

### `POST /api/desktop/save-file`

Opens the native save dialog and writes text content from the desktop host.

### `POST /api/desktop/download-to-path`

Opens the native save dialog and downloads a URL directly from the desktop host.

### `POST /api/desktop/open-path`

Opens a local file or directory in the host OS.

### `POST /api/desktop/reveal-path`

Reveals a local path by opening its containing directory.

### `POST /api/desktop/open-config-dir`

Opens the resolved Kite config directory.

### `POST /api/desktop/open-logs-dir`

Opens the resolved Kite logs directory.

### `POST /api/desktop/window/focus`

Focuses and restores the main window.

### `POST /api/desktop/window/hide`

Hides the main window.

### `POST /api/desktop/window/quit`

Quits the desktop app.

### `GET /api/desktop/window/state?windowName=...`

Returns the live window state of the requested window (defaults to the main
window):

```json
{
  "maximised": false,
  "fullscreen": false
}
```

The host also emits a `kite:window-state-change` DOM `CustomEvent` (with the
same shape as `detail`) on every tracked window whenever it enters or leaves
the maximised / fullscreen state, and on window resizes (fullscreen
transitions resize the window).

### `POST /api/desktop/copy-to-clipboard`

Copies text via the native clipboard.

### `POST /api/desktop/import-kubeconfig`

Imports kubeconfig content, or opens the native file dialog when content is omitted.

## UI Rules

The UI should not directly depend on browser-only behavior for desktop actions.

Desktop-sensitive actions must go through `ui/src/lib/desktop.ts`, including:

- opening external links
- saving files
- downloading files
- opening config or logs directories
- clipboard copy when native bridge is available

## Data Directories

Desktop host ensures these directories exist before runtime boot:

- `Kite/`
- `Kite/logs/`
- `Kite/cache/`
- `Kite/tmp/`

Default SQLite path:

- `Kite/kite.db`
