import { request } from './http'
import { accountPath } from './accounts'
import type { Wallet } from '../types/api'
export const getTickets = (id: string) =>
  request<{
    wallets: Wallet[]
    amount_unit: 'fen'
    currency: 'CNY'
    has_more: boolean
  }>(`${accountPath(id)}/tickets`)
