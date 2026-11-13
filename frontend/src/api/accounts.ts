import { request } from './http'
import type { Account, Status } from '../types/api'
export const accountPath = (id: string) => `/accounts/${encodeURIComponent(id)}`
export const getStatus = () => request<Status>('/status')
export const getAccounts = () =>
  request<{
    accounts: Account[]
  }>('/accounts')
export const checkAccount = (id: string) => request(`${accountPath(id)}/check`, {})
export const removeAccount = (id: string) => request(accountPath(id), undefined, 'DELETE')
export const sendSMS = (phone: string) =>
  request<{
    session_id: string
    retry_after: number
  }>('/login/sms/send', { phone })
export const confirmSMS = (sessionID: string, code: string) =>
  request('/login/sms/confirm', { session_id: sessionID, code })
