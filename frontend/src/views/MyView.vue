<script setup lang="ts">
import { useDashboard } from '../stores/dashboard'
import { maskPhone } from '../utils/format'
import UiIcon from '../components/UiIcon.vue'
import { aboutPages, type AboutPage } from '../config/about'
const { state, currentAccount, displayName } = useDashboard()
const emit = defineEmits<{
  account: []
  connect: []
  open: [page: AboutPage]
}>()
</script>

<template>
  <section class="my-page">
    <button
      class="profile-header"
      aria-label="账号资料"
      @click="currentAccount ? emit('account') : emit('connect')"
    >
      <span class="profile-avatar">
        <UiIcon name="user" :size="38" />
      </span>
      <span class="profile-copy">
        <strong>{{ currentAccount ? displayName : '未登录' }}</strong>
        <span>{{
          currentAccount
            ? currentAccount.phone
              ? '账号：' + maskPhone(currentAccount.phone)
              : '微信授权账号'
            : '点击连接你的账号'
        }}</span>
        <small v-if="currentAccount" :class="{ connected: state.accountConnected }"
          ><i />{{ state.accountConnected ? '已连接' : '已保存' }}</small
        >
      </span>
      <UiIcon name="chevron" :size="20" />
    </button>
    <nav class="my-menu" aria-label="更多信息">
      <button
        v-for="item in aboutPages"
        :key="item.id"
        class="my-menu-item"
        @click="emit('open', item.id)"
      >
        <span class="my-menu-icon"><UiIcon :name="item.icon" :size="20" /></span>
        <span class="my-menu-copy"
          ><strong>{{ item.title }}</strong
          ><small>{{ item.subtitle }}</small></span
        >
        <UiIcon name="chevron" :size="17" />
      </button>
    </nav>
    <p class="my-version">
      水到渠成<span v-if="state.status?.version">
        · v{{ state.status.version.replace(/^v/, '') }}</span
      >
    </p>
  </section>
</template>
