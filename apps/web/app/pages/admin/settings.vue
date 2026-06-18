<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Flame, RefreshCw, Server } from 'lucide-vue-next'

import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { FIREPOWER_KEYS, type PlatformSetting, type PlatformSettingResp, type PlatformSettingsListResp } from '~/types/platform-settings'

definePageMeta({ layout: 'admin' })

const { t } = useI18n()
useHead({ title: () => `Settings - ${t('admin.section')} - ${t('app.name')}` })

setBreadcrumbs(() => [
  { label: t('admin.section'), to: '/admin/settings' },
  { label: 'Settings' },
])

const items = ref<PlatformSetting[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')

const enabled = computed(() => settingValue(FIREPOWER_KEYS.enabled) === 'true')
const runnerMaxTasks = computed(() => settingValue(FIREPOWER_KEYS.runnerMaxTasks) || '64')
const firepowerRunnerMaxTasks = computed(() => settingValue(FIREPOWER_KEYS.firepowerRunnerMaxTasks) || '256')

function settingValue(key: string) {
  return items.value.find(item => item.key === key)?.value ?? ''
}

function upsertLocal(key: string, value: string) {
  const existing = items.value.find(item => item.key === key)
  if (existing) {
    existing.value = value
    return
  }
  items.value.push({ key, value, description: '', updated_at: '' })
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await $fetch<PlatformSettingsListResp>('/api/admin/platform-settings', { credentials: 'include' })
    items.value = res.items ?? []
  } catch (e: any) {
    error.value = e?.data?.error ?? 'Failed to load platform settings'
  } finally {
    loading.value = false
  }
}

async function patchSetting(key: string, value: string) {
  saving.value = true
  error.value = ''
  const oldValue = settingValue(key)
  upsertLocal(key, value)
  try {
    const res = await $fetch<PlatformSettingResp>(`/api/admin/platform-settings/${key}`, {
      method: 'PATCH',
      credentials: 'include',
      body: { value },
    })
    upsertLocal(res.key, res.value)
  } catch (e: any) {
    upsertLocal(key, oldValue)
    error.value = e?.data?.error ?? 'Failed to save platform setting'
  } finally {
    saving.value = false
  }
}

async function onToggleFirepower(value: boolean) {
  await patchSetting(FIREPOWER_KEYS.enabled, value ? 'true' : 'false')
}

async function onBlurInt(key: string, event: FocusEvent) {
  const value = (event.target as HTMLInputElement).value.trim()
  if (!value || value === settingValue(key)) return
  await patchSetting(key, value)
}

onMounted(load)
</script>

<template>
  <div class="space-y-6">
    <header class="space-y-1">
      <h1 class="text-2xl font-semibold tracking-tight">Platform Settings</h1>
      <p class="text-sm text-muted-foreground">
        Persisted global controls for temporary operator modes and runner throughput.
      </p>
    </header>

    <p v-if="error" class="text-sm text-destructive">{{ error }}</p>

    <Card>
      <CardHeader>
        <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div class="space-y-1">
            <CardTitle class="flex items-center gap-2">
              <Flame class="size-5 text-amber-500" />
              Temporary Firepower Mode
            </CardTitle>
            <CardDescription>
              When enabled, fast/worker/reviewer/scout roles and deepseek-named model groups prefer gpt-5.5.
            </CardDescription>
          </div>
          <Badge :variant="enabled ? 'secondary' : 'outline'">
            {{ enabled ? 'Enabled' : 'Disabled' }}
          </Badge>
        </div>
      </CardHeader>
      <CardContent class="space-y-5">
        <div class="flex items-center justify-between gap-4 rounded-lg border p-4">
          <div>
            <Label for="firepower-enabled" class="text-sm font-medium">Firepower enabled</Label>
            <p class="mt-1 text-xs text-muted-foreground">
              Stored as <code class="font-mono">{{ FIREPOWER_KEYS.enabled }}</code>; default is off.
            </p>
          </div>
          <Switch id="firepower-enabled" :checked="enabled" :disabled="loading || saving" @update:checked="onToggleFirepower" />
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <div class="space-y-2">
            <Label for="runner-max" class="flex items-center gap-2 text-xs">
              <Server class="size-3.5" />
              Normal runner task poll limit
            </Label>
            <Input
              id="runner-max"
              :model-value="runnerMaxTasks"
              type="number"
              min="1"
              :disabled="loading || saving"
              @blur="(event: FocusEvent) => onBlurInt(FIREPOWER_KEYS.runnerMaxTasks, event)"
            />
            <p class="text-[11px] text-muted-foreground">Current default behavior remains 64 unless changed.</p>
          </div>

          <div class="space-y-2">
            <Label for="firepower-runner-max" class="flex items-center gap-2 text-xs">
              <Flame class="size-3.5" />
              Firepower runner task poll limit
            </Label>
            <Input
              id="firepower-runner-max"
              :model-value="firepowerRunnerMaxTasks"
              type="number"
              min="1"
              :disabled="loading || saving"
              @blur="(event: FocusEvent) => onBlurInt(FIREPOWER_KEYS.firepowerRunnerMaxTasks, event)"
            />
            <p class="text-[11px] text-muted-foreground">Used only while firepower mode is enabled.</p>
          </div>
        </div>

        <div class="flex items-center gap-2 text-xs text-muted-foreground">
          <RefreshCw class="size-3.5" />
          Changes persist in platform settings and are picked up by backend reads without manual database edits.
        </div>

        <p v-if="loading" class="text-sm text-muted-foreground">{{ t('common.loading') }}</p>
      </CardContent>
    </Card>
  </div>
</template>
