import type { Device, Service, UsageOptions } from '../types/api'
export const maskPhone = (text = '') => text.replace(/(1\d{2})\d{4}(\d{4})/g, '$1****$2')
export function formatAmount(text: string) {
  // Wallet amounts from tokenCoin are fen; service rates already use yuan.
  const value = Number(text) / 100
  return text && Number.isFinite(value) ? value.toFixed(2) : '--'
}
export const categoryLabel = (d: Device) =>
  (({ '04': '饮水机', '08': '淋浴' }) as Record<string, string>)[d.categoryCode] ?? '设备'
export const isIdle = (d: Device) => d.workStatus === 10
export const workStateLabel = (d: Device) =>
  (({ 10: '空闲', 30: '故障', 40: '停用' }) as Record<number, string>)[d.workStatus] ??
  (d.workStatus === 0 ? '待确认' : '使用中')
export function primaryService(options?: UsageOptions) {
  const list = options?.device.services
  return list?.find((s) => s.defaultModel && s.scanAvailable) ?? list?.find((s) => s.scanAvailable)
}
export const formatRate = (s?: Service) =>
  s?.billing_levels?.length && s.billing_unit
    ? `${s.billing_levels.map((l) => l.price).join(' / ')} ${s.billing_unit}`
    : '查看服务'
