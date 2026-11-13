<script setup lang="ts">
import UiIcon from './UiIcon.vue'
const active = defineModel<'home' | 'my'>({ required: true })
</script>

<template>
  <nav class="bottom-nav" aria-label="主导航">
    <button
      :class="{ active: active === 'home' }"
      :aria-current="active === 'home' ? 'page' : undefined"
      @click="active = 'home'"
    >
      <UiIcon name="home" :size="25" />
      <span>首页</span>
    </button>
    <button
      :class="{ active: active === 'my' }"
      :aria-current="active === 'my' ? 'page' : undefined"
      @click="active = 'my'"
    >
      <UiIcon name="user" :size="25" />
      <span>我的</span>
    </button>
  </nav>
</template>
