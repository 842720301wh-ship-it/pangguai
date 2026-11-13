import { request } from './http'
import { accountPath } from './accounts'
import type { Device, UsageOptions, PreparedUsage, UsageResult, LiveUsage } from '../types/api'
import type { ActualLocation } from '../services/phone'
const usagePath = (accountID: string, deviceID: string) =>
  `${accountPath(accountID)}/devices/${encodeURIComponent(deviceID)}/usage`
export const getDevices = (id: string) =>
  request<{
    devices: Device[]
  }>(`${accountPath(id)}/devices`)
export const getUsageOptions = (accountID: string, deviceID: string) =>
  request<UsageOptions>(`${usagePath(accountID, deviceID)}/options`)
export const prepareUsage = (
  accountID: string,
  deviceID: string,
  skuID: string,
  position: ActualLocation,
) =>
  request<PreparedUsage>(`${usagePath(accountID, deviceID)}/prepare`, {
    sku_id: skuID,
    latitude: position.latitude,
    longitude: position.longitude,
    coordinate_system: position.coordinate_system,
  })
export const addScannedDevice = (id: string, text: string) =>
  request<{ device: Device }>(`${accountPath(id)}/devices/scan`, { text })
export const startUsage = (accountID: string, deviceID: string, key: string) =>
  request<UsageResult>(`${usagePath(accountID, deviceID)}/start`, {
    confirmation_key: key,
    confirm: true,
  })
export const getUsageState = (accountID: string, deviceID: string) =>
  request<LiveUsage>(`${usagePath(accountID, deviceID)}/state`)
export const stopUsage = (accountID: string, deviceID: string, session: string) =>
  request<UsageResult>(`${usagePath(accountID, deviceID)}/stop`, {
    session_id: session,
    confirm: true,
  })
