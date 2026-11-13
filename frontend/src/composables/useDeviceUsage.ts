import { ElMessage } from 'element-plus'
import { prepareUsage, startUsage, stopUsage } from '../api/devices'
import { ApiError } from '../api/http'
import { useDashboard } from '../stores/dashboard'
import { locate } from '../services/phone'
import type { Device } from '../types/api'
import { primaryService } from '../utils/format'

export function useDeviceUsage() {
  const dashboard = useDashboard()
  async function start(d: Device) {
    const accountID = dashboard.state.accountID
    const options = dashboard.state.options[d.goodsId]
    const sku =
      options?.device.services.find((s) => s.skuId === dashboard.state.selections[d.goodsId]) ??
      primaryService(options)
    if (dashboard.state.busy) return
    if (!sku?.scanAvailable || !options?.start_supported) {
      ElMessage.info('服务暂不可用，请刷新设备信息')
      return
    }
    dashboard.state.busy = true
    const progress = ElMessage({ message: '正在获取当前位置…', duration: 0 })
    try {
      const before = JSON.stringify(sku)
      const position = await locate()
      progress.close()
      if (accountID !== dashboard.state.accountID) return
      const prepared = await prepareUsage(accountID, d.goodsId, sku.skuId, position)
      if (JSON.stringify(prepared.service) !== before) {
        const stored = dashboard.state.options[d.goodsId]
        if (stored)
          stored.device.services = stored.device.services.map((s) =>
            s.skuId === sku.skuId ? prepared.service : s,
          )
        ElMessage.warning('服务或费率有变化，请核对后再次点击启动')
        return
      }
      // Hide the start action before sending. A late list response must not
      // turn an accepted/ambiguous command back into an idle start button.
      dashboard.setUsageState(d.goodsId, {
        session_id: prepared.confirmation_key,
        state: 'starting',
        can_stop: false,
        started_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        cost_yuan: null,
        cost_source: 'unavailable',
        service_name: sku.name,
      })
      const result = await startUsage(accountID, d.goodsId, prepared.confirmation_key)
      const local = dashboard.state.usage[d.goodsId]
      if (local)
        dashboard.setUsageState(d.goodsId, {
          ...local,
          session_id: result.session_id ?? local.session_id,
          started_at: result.started_at ?? local.started_at,
        })
      ElMessage({
        message: result.message,
        type: result.status === 'accepted' ? 'success' : 'warning',
        duration: 6000,
      })
      await dashboard.loadAccount()
    } catch (e) {
      if (
        e instanceof ApiError &&
        ['usage_already_attempted', 'usage_result_unknown'].includes(e.code ?? '')
      ) {
        dashboard.setUsageState(d.goodsId, {
          ...(dashboard.state.usage[d.goodsId] ?? {
            cost_yuan: null,
            cost_source: 'unavailable',
            updated_at: new Date().toISOString(),
          }),
          state: 'unknown',
          can_stop: false,
        })
      }
      ElMessage.error(e instanceof Error ? e.message : '操作未完成')
      await dashboard.refreshUsage()
    } finally {
      progress.close()
      dashboard.state.busy = false
    }
  }
  async function stop(d: Device) {
    const account = dashboard.state.accountID,
      live = dashboard.state.usage[d.goodsId]
    if (dashboard.state.busy || !live?.session_id || !live.can_stop || live.stale) return
    dashboard.state.busy = true
    dashboard.setUsageState(d.goodsId, { ...live, state: 'stopping', can_stop: false })
    try {
      const result = await stopUsage(account, d.goodsId, live.session_id)
      ElMessage({ message: result.message, type: 'info', duration: 5000 })
    } catch (e) {
      ElMessage.warning(e instanceof Error ? e.message : '终止状态待确认，请核对设备')
    } finally {
      dashboard.state.busy = false
      await dashboard.refreshUsage()
      await dashboard.refreshDevices()
    }
  }
  return { start, stop }
}
