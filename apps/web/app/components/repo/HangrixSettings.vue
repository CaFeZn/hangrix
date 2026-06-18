<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { AlertTriangle, Bot, ExternalLink, FileCode2, RefreshCw } from 'lucide-vue-next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { BlobResp, HangrixBootstrapResp, HangrixTemplate } from '~/types/repo'

const props = defineProps<{
  owner: string
  name: string
  defaultBranch: string
}>()

const { t } = useI18n()

const loading = ref(false)
const configured = ref(false)
const preview = ref('')
const loadError = ref<string | null>(null)
const hostYamlError = ref<string | null>(null)
const submitError = ref<string | null>(null)
const success = ref<string | null>(null)
const applying = ref(false)
const template = ref<HangrixTemplate>('default')

const agentsBlobUrl = computed(() => `/${props.owner}/${props.name}/blob/${props.defaultBranch}/.hangrix/agents.yml`)
const agentsEditUrl = computed(() => `/${props.owner}/${props.name}/edit/${props.defaultBranch}/.hangrix/agents.yml`)
const maintainerEditUrl = computed(() => `/${props.owner}/${props.name}/edit/${props.defaultBranch}/.hangrix/agents/maintainer.md`)

function decodeBlob(b: BlobResp): string {
  const raw = atob(b.content_base64)
  const bytes = new Uint8Array(raw.length)
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i)
  return new TextDecoder('utf-8', { fatal: false }).decode(bytes)
}

async function loadStatus() {
  loading.value = true
  loadError.value = null
  hostYamlError.value = null
  success.value = null
  try {
    const blob = await $fetch<BlobResp>(`/api/repos/${props.owner}/${props.name}/blob`, {
      credentials: 'include',
      query: { ref: props.defaultBranch, path: '.hangrix/agents.yml' },
    })
    configured.value = !blob.binary
    preview.value = blob.binary ? '' : decodeBlob(blob)
  } catch (e: any) {
    const code = e?.statusCode ?? e?.response?.status
    if (code === 404) {
      configured.value = false
      preview.value = ''
    } else {
      loadError.value = e?.data?.error ?? t('repo.hangrix.loadFailed')
      configured.value = false
      preview.value = ''
    }
  } finally {
    loading.value = false
  }

  try {
    const mention = await $fetch<{ host_yaml_error?: string }>(
      `/api/repos/${props.owner}/${props.name}/mention-suggestions`,
      { credentials: 'include' },
    )
    hostYamlError.value = mention.host_yaml_error ? mention.host_yaml_error : null
  } catch {
    hostYamlError.value = null
  }
}

async function applyStarter() {
  submitError.value = null
  success.value = null
  if (configured.value) {
    const ok = window.confirm(t('repo.hangrix.reapplyConfirm'))
    if (!ok) return
  }
  applying.value = true
  try {
    const resp = await $fetch<HangrixBootstrapResp>(`/api/repos/${props.owner}/${props.name}/hangrix/bootstrap`, {
      method: 'POST',
      credentials: 'include',
      body: {
        template: template.value,
      },
    })
    success.value = t('repo.hangrix.applySuccess', { sha: resp.commit.sha.slice(0, 8), branch: resp.branch })
    await loadStatus()
  } catch (e: any) {
    submitError.value = e?.data?.error ?? t('repo.hangrix.applyFailed')
  } finally {
    applying.value = false
  }
}

const previewLines = computed(() => {
  const lines = preview.value.split('\n').slice(0, 16).join('\n').trim()
  return lines
})

watch(() => props.defaultBranch, () => {
  if (props.defaultBranch) loadStatus()
})

onMounted(() => {
  if (props.defaultBranch) loadStatus()
})
</script>

<template>
  <Card>
    <CardHeader>
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="space-y-1">
          <CardTitle class="flex items-center gap-2">
            <Bot class="size-5" />
            {{ t('repo.hangrix.title') }}
          </CardTitle>
          <CardDescription>{{ t('repo.hangrix.subtitle') }}</CardDescription>
        </div>
        <Badge :variant="configured ? 'default' : 'secondary'">
          {{ configured ? t('repo.hangrix.statusConfigured') : t('repo.hangrix.statusMissing') }}
        </Badge>
      </div>
    </CardHeader>
    <CardContent class="space-y-4">
      <p v-if="loadError" class="text-sm text-destructive">
        {{ loadError }}
      </p>
      <div
        v-if="hostYamlError"
        class="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive"
      >
        <p class="flex items-start gap-2">
          <AlertTriangle class="mt-0.5 size-4 shrink-0" />
          <span>{{ t('repo.hangrix.hostYamlError') }}: {{ hostYamlError }}</span>
        </p>
      </div>
      <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
        <div class="space-y-2">
          <Label>{{ t('repo.hangrix.template') }}</Label>
          <Select :model-value="template" @update:model-value="(v) => template = (v as HangrixTemplate) || 'default'">
            <SelectTrigger class="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="default">{{ t('repo.hangrix.templateDefault') }}</SelectItem>
              <SelectItem value="docs">{{ t('repo.hangrix.templateDocs') }}</SelectItem>
            </SelectContent>
          </Select>
          <p class="text-xs text-muted-foreground">
            {{ t('repo.hangrix.templateHint') }}
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <Button variant="outline" :disabled="loading" @click="loadStatus">
            <RefreshCw class="size-4" />
            {{ t('repo.hangrix.refresh') }}
          </Button>
          <Button :disabled="applying || loading" @click="applyStarter">
            {{ applying ? t('repo.hangrix.applying') : (configured ? t('repo.hangrix.reapply') : t('repo.hangrix.apply')) }}
          </Button>
        </div>
      </div>

      <div class="rounded-md border bg-muted/30 p-3 text-sm text-muted-foreground">
        {{ t('repo.hangrix.note') }}
      </div>

      <div v-if="configured" class="space-y-3">
        <div class="flex flex-wrap gap-2">
          <NuxtLink :to="agentsBlobUrl">
            <Button variant="outline" size="sm">
              <FileCode2 class="size-4" />
              {{ t('repo.hangrix.viewAgents') }}
            </Button>
          </NuxtLink>
          <NuxtLink :to="agentsEditUrl">
            <Button variant="outline" size="sm">
              <ExternalLink class="size-4" />
              {{ t('repo.hangrix.editAgents') }}
            </Button>
          </NuxtLink>
          <NuxtLink :to="maintainerEditUrl">
            <Button variant="outline" size="sm">
              <ExternalLink class="size-4" />
              {{ t('repo.hangrix.editMaintainer') }}
            </Button>
          </NuxtLink>
        </div>

        <div class="space-y-2">
          <Label>{{ t('repo.hangrix.preview') }}</Label>
          <pre class="max-h-80 overflow-auto rounded-md border bg-background p-3 text-xs leading-5">{{ previewLines }}</pre>
        </div>
      </div>

      <p v-if="success" class="text-sm text-emerald-600">
        {{ success }}
      </p>
      <p v-if="submitError" class="text-sm text-destructive">
        {{ submitError }}
      </p>
    </CardContent>
  </Card>
</template>
