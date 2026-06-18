import { onMounted, onUnmounted, ref } from 'vue'

interface AdminRefreshOptions {
  intervalMs?: number
  refreshOnVisible?: boolean
}

const DEFAULT_INTERVAL_MS = 10_000

export function useAdminRefresh(
  refresh: () => Promise<void> | void,
  options: AdminRefreshOptions = {},
) {
  const refreshing = ref(false)
  const intervalMs = options.intervalMs ?? DEFAULT_INTERVAL_MS
  const refreshOnVisible = options.refreshOnVisible ?? true
  let timer: ReturnType<typeof setInterval> | null = null
  let pendingManualRefresh = false
  let disposed = false

  async function run(skipWhenHidden: boolean, queueIfBusy = false) {
    if (disposed) return
    if (refreshing.value) {
      if (queueIfBusy) pendingManualRefresh = true
      return
    }
    if (skipWhenHidden && typeof document !== 'undefined' && document.hidden) return

    refreshing.value = true
    try {
      await refresh()
    } finally {
      refreshing.value = false
      if (pendingManualRefresh && !disposed) {
        pendingManualRefresh = false
        void run(skipWhenHidden, false)
      }
    }
  }

  function refreshNow() {
    return run(false, true)
  }

  function refreshIfVisible() {
    return run(true)
  }

  function start() {
    if (timer || typeof window === 'undefined' || intervalMs <= 0) return
    timer = setInterval(() => {
      void refreshIfVisible()
    }, intervalMs)
  }

  function stop() {
    if (!timer) return
    clearInterval(timer)
    timer = null
  }

  function onVisibilityChange() {
    if (!refreshOnVisible || typeof document === 'undefined' || document.hidden) return
    void refreshNow()
  }

  onMounted(() => {
    disposed = false
    start()
    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', onVisibilityChange)
    }
  })

  onUnmounted(() => {
    disposed = true
    pendingManualRefresh = false
    stop()
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  })

  return {
    refreshing,
    refreshNow,
    refreshIfVisible,
  }
}
