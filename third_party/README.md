# Patched dependencies

## wails

Local copy of `github.com/wailsapp/wails/v2` with fixes for Linux StartHidden:

- **Bug**: With `StartHidden: true`, the window flashes briefly on launch.
- **Fixes** in `internal/frontend/desktop/linux/`:
  1. Create window at 1×1 pixel when StartHidden (NewWindow)
  2. Opacity 0 + position (-10000,-10000) before show_all
  3. Hide() after show_all, restore opacity to 1
  4. Show(): resize to full size before showing (from tray)

This patch can be removed when [wailsapp/wails#4882](https://github.com/wailsapp/wails/issues/4882) or a similar fix is merged upstream.
