<script setup lang="ts">
import { computed, ref, watch, onUnmounted } from 'vue'
import type { Device, UsageOptions, LiveUsage } from '../types/api'
import { isIdle, workStateLabel, categoryLabel, primaryService, formatRate } from '../utils/format'
import UiIcon from './UiIcon.vue'
const props = defineProps<{
  device: Device
  options?: UsageOptions
  busy: boolean
  selected?: string
  usage?: LiveUsage
}>()
const emit = defineEmits<{
  open: [device: Device]
  select: [deviceID: string, skuID: string]
}>()
const services = computed(() => props.options?.device.services.filter((s) => s.scanAvailable) ?? [])
const activeService = computed(
  () => services.value.find((s) => s.skuId === props.selected) ?? primaryService(props.options),
)
const active = computed(
  () => !!props.usage && ['starting', 'running', 'stopping', 'unknown'].includes(props.usage.state),
)
const idle = computed(() => !active.value && isIdle(props.device))
const stopReady = computed(() => props.usage?.can_stop && !props.usage.stale)
const action = computed(() =>
  stopReady.value
    ? '终止'
    : active.value
      ? props.usage?.state === 'unknown'
        ? '核对'
        : props.usage?.state === 'running'
          ? '查看'
          : '同步中'
      : idle.value
        ? !props.options
          ? '刷新'
          : '启动'
        : '查看',
)
const status = computed(() =>
  active.value
    ? { starting: '启动中', running: '使用中', stopping: '结束中', unknown: '待核对' }[
        props.usage!.state as 'starting' | 'running' | 'stopping' | 'unknown'
      ]
    : workStateLabel(props.device),
)
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined
watch(
  active,
  (value) => {
    if (timer) clearInterval(timer)
    if (value)
      timer = setInterval(() => {
        now.value = Date.now()
      }, 1000)
  },
  { immediate: true },
)
onUnmounted(() => {
  if (timer) clearInterval(timer)
})
const duration = computed(() => {
  const start = Date.parse(props.usage?.started_at ?? '')
  if (!Number.isFinite(start) || start <= 0 || start > now.value + 5000) return '--:--'
  const seconds = Math.max(0, Math.floor((now.value - start) / 1000))
  return [Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60, seconds % 60]
    .map((v) => String(v).padStart(2, '0'))
    .join(':')
})
const cost = computed(() =>
  props.usage?.cost_source === 'platform' && props.usage.cost_yuan !== null
    ? `¥${Number(props.usage.cost_yuan).toFixed(2)}`
    : '待结算',
)
</script>

<template>
  <article class="device-card">
    <div class="device-content">
      <div class="device-status">
        <UiIcon name="activity" :size="15" />
        <span>状态</span>
        <span class="status-pill" :class="{ unavailable: !idle }"><i />{{ status }}</span>
      </div>
      <div class="device-identity">
        <span class="device-icon">
          <UiIcon :name="device.categoryCode === '08' ? 'shower' : 'water'" :size="26" />
        </span>
        <div>
          <h3>{{ device.goodsName }}</h3>
          <div class="device-tags">
            <span>{{ categoryLabel(device) }}</span>
            <span v-if="device.orgName">{{ device.orgName }}</span>
            <span
              v-if="active && usage?.service_name && usage.service_name !== categoryLabel(device)"
              >{{ usage.service_name }}</span
            >
          </div>
        </div>
      </div>
      <div class="device-facts">
        <template v-if="active">
          <div>
            <span>{{ usage?.state === 'unknown' ? '距启动请求' : '本次时长' }}</span
            ><strong class="usage-duration">{{ duration }}</strong>
          </div>
          <div>
            <span>本次费用</span><strong class="usage-cost">{{ cost }}</strong>
          </div>
        </template>
        <template v-else>
          <div>
            <span>费率</span>
            <strong>{{ formatRate(activeService) }}</strong>
          </div>
          <div>
            <span>服务</span>
            <div v-if="services.length > 1" class="inline-services" aria-label="选择服务">
              <button
                v-for="s in services"
                :key="s.skuId"
                :aria-pressed="s.skuId === activeService?.skuId"
                :disabled="busy"
                @click="emit('select', device.goodsId, s.skuId)"
              >
                {{ s.name }}
              </button>
            </div>
            <strong v-else>{{ activeService?.name ?? categoryLabel(device) }}</strong>
          </div>
        </template>
      </div>
      <p v-if="active" class="device-live-note">
        {{
          usage?.stale
            ? '状态同步中，请稍候'
            : usage?.state === 'unknown'
              ? usage.note || '使用结果待核对，请检查官方 App'
              : usage?.state === 'starting'
                ? '正在确认设备启动'
                : usage?.state === 'stopping'
                  ? '正在确认结束，请勿重复操作'
                  : usage?.stop_supported === false
                    ? '本设备不支持远程终止'
                    : usage?.cost_source === 'platform'
                      ? '费用由平台回传'
                      : '实时金额暂未回传，以平台结算为准'
        }}
      </p>
    </div>
    <div class="device-control">
      <button
        class="round-action"
        :aria-label="`${action} ${device.goodsName}`"
        :disabled="
          busy ||
          !usage ||
          (active && !stopReady && usage?.state !== 'unknown' && usage?.state !== 'running')
        "
        @click="emit('open', device)"
      >
        <UiIcon :name="stopReady ? 'stop' : idle ? 'play' : 'activity'" :size="27" />
      </button>
      <span>{{ action }}</span>
    </div>
  </article>
</template>
