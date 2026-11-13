<script setup lang="ts">
import UiIcon from './UiIcon.vue'

defineProps<{ title: string; scan?: boolean; back?: boolean; busy: boolean }>()
const emit = defineEmits<{ scan: []; back: [] }>()
</script>

<template>
  <header class="app-navbar" aria-label="顶部导航">
    <div class="app-navbar__side">
      <button v-if="back" class="icon-button" aria-label="返回我的" @click="emit('back')">
        <UiIcon name="back" :size="21" />
      </button>
    </div>
    <h1 class="app-navbar__title">{{ title }}</h1>
    <div class="app-navbar__side app-navbar__side--right">
      <button
        v-if="scan"
        class="icon-button"
        :disabled="busy"
        aria-label="扫码添加设备"
        @click="emit('scan')"
      >
        <UiIcon name="scan" :size="23" />
      </button>
    </div>
  </header>
</template>
