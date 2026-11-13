<script setup lang="ts">
import { useDashboard } from '../stores/dashboard'
import type { Device } from '../types/api'
import UiIcon from '../components/UiIcon.vue'
import TicketWallet from '../components/TicketWallet.vue'
import DeviceCard from '../components/DeviceCard.vue'
const { state, currentAccount, loadAccount, refresh, selectService } = useDashboard()
const emit = defineEmits<{
  scan: []
  connect: []
  openDevice: [device: Device]
}>()
</script>

<template>
  <section class="home-page">
    <TicketWallet
      :wallets="state.wallets"
      :connected="!!currentAccount"
      :loading="state.loading"
      :error="state.walletError"
      @refresh="loadAccount"
      @connect="emit('connect')"
    />
    <div class="device-heading">
      <h2>设备</h2>
      <span v-if="state.devices.length">{{ state.devices.length }} 台</span>
    </div>
    <div v-if="state.loading && !state.devices.length" class="empty-state" role="status">
      正在读取设备…
    </div>
    <div v-else-if="state.loadError" class="empty-state">
      <p>{{ state.loadError }}</p>
      <button class="subtle-button" @click="refresh">重新连接</button>
    </div>
    <div v-else-if="!currentAccount" class="empty-state">
      <UiIcon name="user" :size="38" />
      <p>连接账号，查看你的设备</p>
      <button class="primary-button" @click="emit('connect')">连接账号</button>
    </div>
    <div v-else-if="!state.devices.length" class="empty-state">
      <UiIcon name="scan" :size="38" />
      <p>还没有设备</p>
      <button class="subtle-button" @click="emit('scan')">扫码添加</button>
    </div>
    <div v-else class="device-list">
      <DeviceCard
        v-for="device in state.devices"
        :key="device.goodsId"
        :device="device"
        :options="state.options[device.goodsId]"
        :busy="state.busy"
        :selected="state.selections[device.goodsId]"
        :usage="state.usage[device.goodsId]"
        @select="selectService"
        @open="emit('openDevice', $event)"
      />
    </div>
  </section>
</template>
