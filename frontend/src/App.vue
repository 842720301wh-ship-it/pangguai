<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'
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
const mainContent = ref<HTMLElement>()
watch([page, aboutPage], () => mainContent.value?.scrollTo(0, 0), { flush: 'post' })
const profileOpen = ref(false),
  loginOpen = ref(false)
const { state, currentAccount, refresh, refreshUsage } = useDashboard(),
  usage = useDeviceUsage()
const checkingUsage = ref(false)
let checkFeedback: ReturnType<typeof ElMessage> | undefined
async function showCheckFeedback(message: string, type: 'info' | 'warning' | 'success') {
  checkFeedback?.close()
  await nextTick()
  checkFeedback = ElMessage({ message, type })
}
async function checkUsage(device: Device) {
  if (checkingUsage.value) return
  checkingUsage.value = true
  const account = state.accountID
  checkFeedback?.close()
  const progress = ElMessage({ message: '正在核对设备状态…', duration: 0 })
  checkFeedback = progress
  try {
    const refreshed = await refreshUsage()
    if (account !== state.accountID) return
    if (!refreshed) {
      await showCheckFeedback('设备状态正在同步，请稍后再核对', 'info')
      return
    }
    const live = state.usage[device.goodsId]
    if (!live || live.stale) {
      await showCheckFeedback(
        '核对失败，暂时无法查询平台状态，请稍后重试或在官方 App 查看',
        'warning',
      )
    } else if (live.state === 'finished' || live.state === 'idle') {
      await showCheckFeedback('本地使用记录已解除，设备列表已刷新', 'success')
    } else if (live.state === 'unknown') {
      await showCheckFeedback(
        live.note || '平台尚未确认本次使用是否结束，请在官方 App 核对',
        'warning',
      )
    } else {
      await showCheckFeedback(
        { starting: '平台仍在确认启动', running: '平台仍显示使用中', stopping: '平台仍在确认结束' }[
          live.state
        ],
        'info',
      )
    }
  } finally {
    progress.close()
    checkingUsage.value = false
  }
}
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
    void checkUsage(device)
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
    <main ref="mainContent" class="app-main">
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
