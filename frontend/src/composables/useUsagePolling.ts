import { onMounted, onUnmounted } from 'vue'
import { useDashboard } from '../stores/dashboard'

export function useUsagePolling() {
  const dashboard = useDashboard()
  let timer: ReturnType<typeof setTimeout> | undefined,
    disposed = false,
    lastDevices = 0,
    generation = 0
  async function tick(current: number) {
    if (disposed || document.hidden || current !== generation) return
    await dashboard.refreshUsage()
    if (disposed || document.hidden || current !== generation) return
    if (Date.now() - lastDevices > 15000) {
      lastDevices = Date.now()
      await dashboard.refreshDevices()
    }
    if (!disposed && !document.hidden && current === generation)
      timer = setTimeout(() => tick(current), 4000)
  }
  function visibility() {
    if (timer) clearTimeout(timer)
    generation++
    if (!document.hidden) void tick(generation)
  }
  onMounted(() => {
    timer = setTimeout(() => tick(generation), 4000)
    document.addEventListener('visibilitychange', visibility)
  })
  onUnmounted(() => {
    disposed = true
    generation++
    if (timer) clearTimeout(timer)
    document.removeEventListener('visibilitychange', visibility)
  })
}
