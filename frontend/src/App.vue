<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useDashboard } from './stores/dashboard'
import { useDeviceUsage } from './composables/useDeviceUsage'
import { isIdle } from './utils/format'
import type { Device } from './types/api'
import HomeView from './views/HomeView.vue'
import MyView from './views/MyView.vue'
import AboutView from './views/AboutView.vue'
import BottomNav from './components/BottomNav.vue'
import AccountSheet from './components/AccountSheet.vue'
import LoginSheet from './components/LoginSheet.vue'
import AppNavBar from './components/AppNavBar.vue'
import { scanCode } from './services/phone'
import { addScannedDevice } from './api/devices'
import { useUsagePolling } from './composables/useUsagePolling'
import { useNavigation } from './composables/useNavigation'
import { aboutPages } from './config/about'
const { page, aboutPage, navigate, backToMy } = useNavigation()
const profileOpen = ref(false),
  loginOpen = ref(false)
const { state, currentAccount, refresh, refreshUsage } = useDashboard(),
  usage = useDeviceUsage()
useUsagePolling()
async function scan() {
  if (state.busy) return
  if (!currentAccount.value) {
    loginOpen.value = true
  } else {
    state.busy = true
    const id = state.accountID
    try {
      const result = await scanCode()
      if (id !== state.accountID) return
      const { device } = await addScannedDevice(id, result.text)
      ElMessage.success(`已添加 ${device.goodsName}`)
      await refresh()
    } catch (e) {
      if (!(e instanceof Error) || e.message !== 'scan_cancelled')
        ElMessage.error(e instanceof Error ? e.message : '扫码未完成')
    } finally {
      state.busy = false
    }
  }
}
function openDevice(device: Device) {
  const live = state.usage[device.goodsId]
  if (live?.can_stop && !live.stale) {
    void usage.stop(device)
  } else if (live?.state === 'running' && live.stop_supported === false) {
    ElMessage.info('本设备不支持远程终止，请使用设备自身的结束方式')
  } else if (live && ['starting', 'running', 'stopping', 'unknown'].includes(live.state)) {
    void refreshUsage()
  } else if (isIdle(device)) {
    void usage.start(device)
  } else if (device.workStatus === 0) {
    ElMessage.info('设备已保存，平台状态待确认，请刷新或在官方 App 查看')
  } else {
    ElMessage.info('该设备当前非空闲，尚未确认是本人使用，请在官方 App 核对')
  }
}
onMounted(refresh)
</script>

<template>
  <div class="app-frame" :class="{ 'my-view': page === 'my', 'about-view': aboutPage }">
    <AppNavBar
      :title="
        aboutPage
          ? aboutPages.find((p) => p.id === aboutPage)!.title
          : page === 'home'
            ? '水到渠成'
            : '我的'
      "
      :scan="page === 'home'"
      :back="!!aboutPage"
      :busy="state.busy"
      @scan="scan"
      @back="backToMy"
    />
    <main>
      <AboutView v-if="aboutPage" :page="aboutPage" @open="navigate" />
      <HomeView
        v-else-if="page === 'home'"
        @scan="scan"
        @connect="loginOpen = true"
        @open-device="openDevice"
      />
      <MyView v-else @account="profileOpen = true" @connect="loginOpen = true" @open="navigate" />
    </main>
    <BottomNav v-if="!aboutPage" :model-value="page" @update:model-value="navigate" />
    <AccountSheet v-model="profileOpen" @connect="loginOpen = true" />
    <LoginSheet v-model="loginOpen" />
  </div>
</template>
