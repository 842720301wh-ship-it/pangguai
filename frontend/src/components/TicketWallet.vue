<script setup lang="ts">
import type { Wallet } from '../types/api'
import { formatAmount } from '../utils/format'
import UiIcon from './UiIcon.vue'
defineProps<{
  wallets: Wallet[]
  connected: boolean
  loading: boolean
  error: string
}>()
const emit = defineEmits<{
  refresh: []
  connect: []
}>()
</script>

<template>
  <section class="ticket-card" aria-label="通用小票余额">
    <h2><UiIcon name="ticket" :size="21" />通用小票</h2>
    <template v-if="connected">
      <div class="wallet-grid">
        <div v-for="wallet in wallets" :key="wallet.shopId" class="wallet-row">
          <span>{{ wallet.shopName }}</span>
          <strong><small>¥</small>{{ formatAmount(wallet.totalAmount) }}</strong>
        </div>
      </div>
      <p v-if="loading && !wallets.length" class="wallet-note">正在读取余额…</p>
      <p v-else-if="error" class="wallet-note">
        {{ error }}<button @click="emit('refresh')">重试</button>
      </p>
      <p v-else-if="!wallets.length" class="wallet-note">当前账号暂无通用小票</p>
    </template>
    <button v-else class="connect-link" @click="emit('connect')">
      连接账号后查看余额<UiIcon name="chevron" :size="17" />
    </button>
  </section>
</template>
