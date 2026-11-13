<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { checkAccount, removeAccount } from '../api/accounts'
import { useDashboard } from '../stores/dashboard'
import { maskPhone } from '../utils/format'
import UiIcon from './UiIcon.vue'
const visible = defineModel<boolean>({ required: true })
const emit = defineEmits<{
  connect: []
}>()
const { state, currentAccount, selectAccount, refresh } = useDashboard()
function connect() {
  visible.value = false
  emit('connect')
}
async function select(id: string) {
  if (state.busy) return
  visible.value = false
  await selectAccount(id)
}
async function check() {
  if (!currentAccount.value || state.busy) return
  state.busy = true
  try {
    await checkAccount(state.accountID)
    state.accountConnected = true
    ElMessage.success('账号连接正常')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '检查失败')
  } finally {
    state.busy = false
  }
}
async function remove() {
  if (!currentAccount.value || state.busy) return
  try {
    await ElMessageBox.confirm('移除本机保存的账号和登录凭据？', '移除账号', {
      confirmButtonText: '移除',
      cancelButtonText: '取消',
    })
    state.busy = true
    await removeAccount(state.accountID)
    visible.value = false
    await refresh()
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message)
  } finally {
    state.busy = false
  }
}
</script>

<template>
  <el-drawer v-model="visible" direction="btt" size="auto" title="账号" class="bottom-sheet">
    <div class="profile-sheet">
      <button
        v-for="account in state.accounts"
        :key="account.id"
        class="account-choice"
        :disabled="state.busy"
        @click="select(account.id)"
      >
        <UiIcon name="user" />
        <span>{{ maskPhone(account.phone || account.name) }}</span>
        <UiIcon v-if="account.id === state.accountID" name="check" :size="18" />
      </button>
      <button class="sheet-row" :disabled="state.busy" @click="check">
        检查连接<UiIcon name="chevron" :size="17" />
      </button>
      <button class="sheet-row" :disabled="state.busy" @click="connect">
        连接其他账号<UiIcon name="chevron" :size="17" />
      </button>
      <button class="sheet-row danger" :disabled="state.busy" @click="remove">移除本机账号</button>
    </div>
  </el-drawer>
</template>
