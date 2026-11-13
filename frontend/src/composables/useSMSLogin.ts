import { onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { sendSMS, confirmSMS } from '../api/accounts'
import { useDashboard } from '../stores/dashboard'
export function useSMSLogin(onComplete: () => void) {
  const { state, refresh } = useDashboard()
  const phone = ref(''),
    code = ref(''),
    consent = ref(false),
    sessionID = ref(''),
    cooldown = ref(0)
  const timer = setInterval(() => {
    if (cooldown.value) cooldown.value--
  }, 1000)
  onUnmounted(() => clearInterval(timer))
  async function send() {
    if (
      !/^1[3-9]\d{9}$/.test(phone.value) ||
      !consent.value ||
      state.busy ||
      cooldown.value ||
      !state.status?.protocol_ready
    )
      return
    state.busy = true
    cooldown.value = 60
    try {
      const reply = await sendSMS(phone.value)
      sessionID.value = reply.session_id
      cooldown.value = reply.retry_after
      ElMessage.success('验证码已发送')
    } catch (e) {
      ElMessage.error(e instanceof Error ? e.message : '发送未确认')
    } finally {
      state.busy = false
    }
  }
  async function login() {
    if (!sessionID.value || !/^\d{4,8}$/.test(code.value) || !consent.value || state.busy) return
    state.busy = true
    try {
      await confirmSMS(sessionID.value, code.value)
      code.value = ''
      phone.value = ''
      sessionID.value = ''
      onComplete()
      await refresh()
      ElMessage.success('账号已连接')
    } catch (e) {
      ElMessage.error(e instanceof Error ? e.message : '连接未完成')
    } finally {
      state.busy = false
    }
  }
  return { phone, code, consent, sessionID, cooldown, send, login }
}
