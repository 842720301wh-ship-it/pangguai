<script setup lang="ts">
import { useSMSLogin } from '../composables/useSMSLogin'
import { useDashboard } from '../stores/dashboard'
const visible = defineModel<boolean>({ required: true })
const { state } = useDashboard()
const { phone, code, consent, sessionID, cooldown, send, login } = useSMSLogin(() => {
  visible.value = false
})
</script>

<template>
  <el-drawer
    v-model="visible"
    direction="btt"
    size="auto"
    title="连接账号"
    class="bottom-sheet"
    @closed="code = ''"
  >
    <form class="login-sheet" @submit.prevent="login">
      <label for="phone">手机号码</label>
      <el-input
        id="phone"
        v-model="phone"
        inputmode="tel"
        autocomplete="tel"
        maxlength="11"
        placeholder="请输入本人手机号"
        :disabled="!!sessionID || state.busy"
      />
      <label for="code">验证码</label>
      <div class="code-row">
        <el-input
          id="code"
          v-model="code"
          type="password"
          inputmode="numeric"
          maxlength="8"
          autocomplete="one-time-code"
          placeholder="短信验证码"
        />
        <button
          type="button"
          :disabled="
            !/^1[3-9]\d{9}$/.test(phone) ||
            !consent ||
            state.busy ||
            cooldown > 0 ||
            !state.status?.protocol_ready
          "
          @click="send"
        >
          {{ cooldown ? `${cooldown}s` : '获取验证码' }}
        </button>
      </div>
      <el-checkbox v-model="consent">本人账号，同意在本机保存登录状态</el-checkbox>
      <button
        class="primary-button full-width"
        type="submit"
        :disabled="!sessionID || !consent || !/^\d{4,8}$/.test(code) || state.busy"
      >
        {{ state.busy ? '正在连接…' : '连接' }}
      </button>
      <p class="sheet-note">验证码仅在点击后发送。微信直接授权暂未接入。</p>
    </form>
  </el-drawer>
</template>
