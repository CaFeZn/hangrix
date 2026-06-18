<script setup lang="ts">
import { computed } from 'vue'
import type { NuxtError } from '#app'
import { Button } from '@/components/ui/button'
import { isChunkLoadFailure, tryRecoverChunkFailure } from './utils/chunk-recovery'

const props = defineProps<{ error: NuxtError }>()

const { t, locale } = useI18n()

const status = computed(() => props.error?.statusCode ?? 500)
const message = computed(() => props.error?.statusMessage || props.error?.message || t('errors.unknown'))
const isChunkError = computed(() => isChunkLoadFailure(props.error))
const retryingMessage = computed(() =>
  locale.value.startsWith('zh')
    ? '正在刷新页面以恢复失效的前端资源...'
    : 'Refreshing to recover from a stale frontend bundle...',
)

if (import.meta.client && isChunkError.value) {
  tryRecoverChunkFailure(props.error)
}

function goHome() {
  clearError({ redirect: '/' })
}
</script>

<template>
  <div class="grid min-h-screen place-items-center bg-background p-6 text-foreground">
    <div class="max-w-md space-y-4 text-center">
      <p class="font-mono text-6xl font-semibold tracking-tighter text-muted-foreground">
        {{ status }}
      </p>
      <h1 class="text-2xl font-semibold tracking-tight">
        {{ message }}
      </h1>
      <p v-if="isChunkError" class="text-sm text-muted-foreground">
        {{ retryingMessage }}
      </p>
      <Button @click="goHome">
        {{ t('nav.home') }}
      </Button>
    </div>
  </div>
</template>
