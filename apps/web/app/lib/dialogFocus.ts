export function dialogFocus() {
  let trigger: HTMLElement | null = null
  return {
    onOpenAutoFocus() {
      const active = document.activeElement
      const menu = active?.closest('[role="menu"]')
      // A selected menu item disappears; restore focus to its stable row trigger.
      trigger = menu ? document.getElementById(menu.getAttribute('aria-labelledby') ?? '') : active as HTMLElement | null
    },
    onCloseAutoFocus(event: Event) {
      if (event.defaultPrevented || !trigger?.isConnected) return
      event.preventDefault()
      trigger.focus()
    },
  }
}
