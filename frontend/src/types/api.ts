export type Identifier = string
export interface Account {
  id: Identifier
  name: string
  phone?: string
  updated_at: string
}
export interface Status {
  app: string
  version: string
  protocol_ready: boolean
  methods: string[]
  message: string
}
export interface Device {
  goodsId: Identifier
  goodsName: string
  categoryCode: string
  workStatus: number
  orgName?: string
}
export interface Wallet {
  shopId: Identifier
  shopName: string
  totalAmount: string
  principalAmount?: string
  presentAmount?: string
}
export interface Service {
  skuId: Identifier
  name: string
  scanAvailable: boolean
  defaultModel?: boolean
  billing_unit?: string
  billing_levels?: {
    price: string | number
  }[]
  [key: string]: unknown
}
export interface UsageOptions {
  device: {
    goods_id: Identifier
    name: string
    category: string
    services: Service[]
  }
  start_supported: boolean
  mode: 'readonly'
}
export interface PreparedUsage {
  status: 'prepared'
  confirmation_key: string
  expires_in: number
  service: Service
  billing: string
}
export interface UsageResult {
  status: string
  message: string
  session_id?: string
  started_at?: string
}
export interface LiveUsage {
  session_id?: string
  state: 'idle' | 'starting' | 'running' | 'stopping' | 'unknown' | 'finished'
  can_stop: boolean
  stop_supported?: boolean
  started_at?: string
  finished_at?: string
  updated_at: string
  service_name?: string
  cost_yuan: string | null
  cost_source: 'platform' | 'unavailable'
  note?: string
  stale?: boolean
}
