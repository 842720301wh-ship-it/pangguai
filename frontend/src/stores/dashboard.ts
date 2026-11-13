import { computed, reactive } from 'vue'
import { getAccounts, getStatus } from '../api/accounts'
import { getDevices, getUsageOptions, getUsageState } from '../api/devices'
import { getTickets } from '../api/tickets'
import type { Account, Device, Wallet, Status, UsageOptions, LiveUsage } from '../types/api'
import { isIdle, maskPhone } from '../utils/format'
const state = reactive({
  accounts: [] as Account[],
  accountID: '',
  status: null as Status | null,
  devices: [] as Device[],
  wallets: [] as Wallet[],
  options: {} as Record<string, UsageOptions>,
  selections: {} as Record<string, string>,
  usage: {} as Record<string, LiveUsage>,
  loading: false,
  busy: false,
  loadError: '',
  walletError: '',
  accountConnected: false,
})
let epoch = 0
const usageRevisions: Record<string, number> = {}
function setUsageState(device: string, value: LiveUsage) {
  usageRevisions[device] = (usageRevisions[device] ?? 0) + 1
  state.usage[device] = value
}
const currentAccount = computed(() => state.accounts.find((a) => a.id === state.accountID))
const displayName = computed(() =>
  currentAccount.value?.name && !/^1\d{10}$/.test(currentAccount.value.name)
    ? maskPhone(currentAccount.value.name)
    : '胖乖用户',
)
async function loadAccount() {
  const version = ++epoch,
    id = state.accountID
  state.devices = []
  state.wallets = []
  state.options = {}
  state.loadError = ''
  state.walletError = ''
  state.accountConnected = false
  state.loading = !!id
  if (!id) return
  const [d, w] = await Promise.allSettled([getDevices(id), getTickets(id)])
  if (version !== epoch) return
  if (w.status === 'fulfilled' && w.value.amount_unit === 'fen') {
    state.wallets = w.value.wallets
    state.accountConnected = true
  } else state.walletError = '暂未读取到小票余额'
  if (d.status === 'fulfilled') {
    state.devices = d.value.devices
    state.accountConnected = true
  } else state.loadError = d.reason instanceof Error ? d.reason.message : '设备读取失败'
  const reads = await Promise.allSettled(
    state.devices
      .filter(isIdle)
      .map(async (d) => [d.goodsId, await getUsageOptions(id, d.goodsId)] as const),
  )
  if (version !== epoch) return
  reads.forEach((r) => {
    if (r.status === 'fulfilled') state.options[r.value[0]] = r.value[1]
  })
  state.loading = false
  await refreshUsage()
}
let usageReading = false
async function refreshUsage() {
  if (usageReading || !state.accountID) return
  usageReading = true
  const id = state.accountID,
    version = epoch
  try {
    await Promise.all(
      state.devices.map(async (d) => {
        const revision = usageRevisions[d.goodsId] ?? 0
        try {
          const live = await getUsageState(id, d.goodsId)
          if (
            id === state.accountID &&
            version === epoch &&
            revision === (usageRevisions[d.goodsId] ?? 0)
          )
            state.usage[d.goodsId] = live
        } catch {
          if (
            id === state.accountID &&
            version === epoch &&
            revision === (usageRevisions[d.goodsId] ?? 0)
          )
            state.usage[d.goodsId] = {
              ...(state.usage[d.goodsId] ?? {
                state: 'unknown',
                cost_yuan: null,
                cost_source: 'unavailable',
                updated_at: new Date().toISOString(),
              }),
              can_stop: false,
              stale: true,
            }
        }
      }),
    )
  } finally {
    usageReading = false
  }
}
async function refreshDevices() {
  const id = state.accountID,
    version = epoch
  if (!id || state.busy) return
  try {
    const result = await getDevices(id)
    if (id === state.accountID && version === epoch) state.devices = result.devices
  } catch {
    /* Preserve the last state; never reset active sessions to idle on failure. */
  }
}
async function refresh() {
  try {
    const [status, { accounts }] = await Promise.all([getStatus(), getAccounts()])
    state.status = status
    state.accounts = accounts
    if (!accounts.some((a) => a.id === state.accountID)) state.accountID = accounts[0]?.id ?? ''
    await loadAccount()
  } catch (e) {
    state.loading = false
    state.loadError = e instanceof Error ? e.message : '连接失败'
  }
}
async function selectAccount(id: string) {
  if (state.busy) return
  state.accountID = id
  state.selections = {}
  state.usage = {}
  await loadAccount()
}
function selectService(id: string, sku: string) {
  if (
    !state.busy &&
    state.options[id]?.device.services.some((s) => s.skuId === sku && s.scanAvailable)
  )
    state.selections[id] = sku
}
export function useDashboard() {
  return {
    state,
    currentAccount,
    displayName,
    refresh,
    loadAccount,
    selectAccount,
    selectService,
    refreshUsage,
    refreshDevices,
    setUsageState,
  }
}
