<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import * as z from 'zod'
import { AlertTriangle, Bot, Check, Copy, Plus, Server, Trash2, X } from 'lucide-vue-next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

import type { Runner, RunnerCreateResp, RunnerListResp } from '~/types/runner'

const props = defineProps<{
  repoOwnerKind: 'user' | 'org'
  repoOwnerName: string
}>()

const { t } = useI18n()
const { user } = useCurrentUser()

const runners = ref<Runner[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

const createOpen = ref(false)
const createError = ref<string | null>(null)

const enrollOpen = ref(false)
const enrollToken = ref('')
const enrollRunner = ref<Runner | null>(null)
const enrollCopied = ref(false)
const oneLinerCopied = ref(false)

const schema = computed(() => toTypedSchema(z.object({
  name: z.string().min(1).max(64).regex(/^[a-z0-9][a-z0-9-]{0,63}$/),
})))
const initial = { name: '' }

const serverOrigin = computed(() => {
  if (typeof window === 'undefined') return ''
  return window.location.origin
})

const installOneLiner = computed(() => {
  if (!enrollToken.value) return ''
  return `curl -fsSL ${serverOrigin.value}/install/runner.sh | sh -s -- ${enrollToken.value}`
})

const manualEnroll = computed(() => {
  if (!enrollToken.value) return ''
  return `hangrix-runner enroll --server ${serverOrigin.value} --token ${enrollToken.value}`
})

const repoEligibilityHint = computed(() => {
  if (props.repoOwnerKind !== 'user') return t('repo.runner.orgScopeHint')
  if (user.value?.username && user.value.username !== props.repoOwnerName) {
    return t('repo.runner.foreignOwnerHint', { owner: props.repoOwnerName })
  }
  return t('repo.runner.scopeHint')
})

function formatDate(s?: string | null) {
  if (!s) return ''
  try { return new Date(s).toLocaleString() } catch { return s }
}

function statusVariant(r: Runner) {
  if (r.status === 'active') return r.online ? 'secondary' : 'destructive'
  if (r.status === 'disabled') return 'destructive'
  return 'outline'
}

function statusLabel(r: Runner) {
  if (r.status === 'active' && !r.online) return t('repo.runner.offline')
  return r.status
}

async function load() {
  loading.value = true
  error.value = null
  try {
    const res = await $fetch<RunnerListResp>('/api/runners', { credentials: 'include' })
    runners.value = res.items ?? []
  } catch (e: any) {
    error.value = e?.data?.error ?? t('repo.runner.loadFailed')
  } finally {
    loading.value = false
  }
}

async function onCreate(values: any, ctx: any) {
  createError.value = null
  try {
    const res = await $fetch<RunnerCreateResp>('/api/runners', {
      method: 'POST',
      credentials: 'include',
      body: { name: values.name },
    })
    enrollToken.value = res.enroll_token
    enrollRunner.value = res.runner
    enrollCopied.value = false
    oneLinerCopied.value = false
    createOpen.value = false
    enrollOpen.value = true
    ctx?.resetForm?.({ values: initial })
    await load()
  } catch (e: any) {
    createError.value = e?.data?.error ?? t('repo.runner.createFailed')
  }
}

async function onDisable(r: Runner) {
  if (!window.confirm(t('repo.runner.disableConfirm', { name: r.name }))) return
  try {
    await $fetch(`/api/runners/${r.id}`, { method: 'DELETE', credentials: 'include' })
    await load()
  } catch (e: any) {
    error.value = e?.data?.error ?? t('repo.runner.disableFailed')
  }
}

async function onRemove(r: Runner) {
  if (!window.confirm(t('repo.runner.removeConfirm', { name: r.name }))) return
  try {
    await $fetch(`/api/runners/${r.id}/permanent`, { method: 'DELETE', credentials: 'include' })
    await load()
  } catch (e: any) {
    error.value = e?.data?.error ?? t('repo.runner.removeFailed')
  }
}

async function copyEnroll() {
  try {
    await navigator.clipboard.writeText(enrollToken.value)
    enrollCopied.value = true
    setTimeout(() => { enrollCopied.value = false }, 1500)
  } catch {}
}

async function copyOneLiner() {
  try {
    await navigator.clipboard.writeText(installOneLiner.value)
    oneLinerCopied.value = true
    setTimeout(() => { oneLinerCopied.value = false }, 1500)
  } catch {}
}

function acknowledge() {
  enrollOpen.value = false
  enrollToken.value = ''
  enrollRunner.value = null
}

onMounted(load)
</script>

<template>
  <div class="space-y-6">
    <Card>
      <CardHeader>
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="space-y-1">
            <CardTitle class="flex items-center gap-2">
              <Server class="size-5" />
              {{ t('repo.runner.title') }}
            </CardTitle>
            <CardDescription>{{ t('repo.runner.subtitle') }}</CardDescription>
          </div>
          <Button @click="createOpen = true">
            <Plus class="size-4" />
            {{ t('repo.runner.create') }}
          </Button>
        </div>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="rounded-md border bg-muted/30 p-3 text-sm text-muted-foreground">
          {{ repoEligibilityHint }}
        </div>

        <div class="rounded-md border bg-muted/10 p-3 text-sm text-muted-foreground">
          <p class="font-medium text-foreground">{{ t('repo.runner.setupTitle') }}</p>
          <p class="mt-1">{{ t('repo.runner.setupHint') }}</p>
          <p class="mt-1 text-xs">{{ t('repo.runner.requirementsHint') }}</p>
        </div>

        <p v-if="error" class="text-sm text-destructive">{{ error }}</p>

        <div v-if="!loading && runners.length === 0" class="rounded-lg border border-dashed p-8 text-center">
          <Bot class="mx-auto size-8 text-muted-foreground" />
          <p class="mt-3 text-sm font-medium">{{ t('repo.runner.empty') }}</p>
          <p class="mt-1 text-xs text-muted-foreground">{{ t('repo.runner.emptyHint') }}</p>
        </div>

        <Table v-else>
          <TableHeader>
            <TableRow>
              <TableHead>{{ t('repo.runner.cols.name') }}</TableHead>
              <TableHead>{{ t('repo.runner.cols.status') }}</TableHead>
              <TableHead>{{ t('repo.runner.cols.enroll') }}</TableHead>
              <TableHead>{{ t('repo.runner.cols.lastHeartbeat') }}</TableHead>
              <TableHead class="text-right">{{ t('common.actions') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="r in runners" :key="r.id">
              <TableCell class="font-medium">{{ r.name }}</TableCell>
              <TableCell><Badge :variant="statusVariant(r)">{{ statusLabel(r) }}</Badge></TableCell>
              <TableCell>
                <Badge v-if="r.enroll_token_used" variant="secondary">{{ t('repo.runner.enrolled') }}</Badge>
                <Badge v-else variant="outline">{{ t('repo.runner.notEnrolled') }}</Badge>
              </TableCell>
              <TableCell class="text-xs text-muted-foreground">{{ formatDate(r.last_heartbeat_at) || t('repo.runner.neverHeartbeat') }}</TableCell>
              <TableCell class="space-x-2 text-right">
                <Button
                  v-if="r.status !== 'disabled'"
                  size="sm"
                  variant="outline"
                  @click="onDisable(r)"
                >
                  <Trash2 class="size-3" />
                  {{ t('repo.runner.disable') }}
                </Button>
                <Button size="sm" variant="destructive" @click="onRemove(r)">
                  <X class="size-3" />
                  {{ t('repo.runner.remove') }}
                </Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <p v-if="loading" class="text-sm text-muted-foreground">{{ t('common.loading') }}</p>
      </CardContent>
    </Card>

    <Dialog v-model:open="createOpen">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ t('repo.runner.createTitle') }}</DialogTitle>
          <DialogDescription>{{ t('repo.runner.createSubtitle') }}</DialogDescription>
        </DialogHeader>
        <Form v-slot="{ isSubmitting }" :validation-schema="schema" :initial-values="initial" keep-values @submit="onCreate">
          <div class="space-y-4">
            <FormField v-slot="{ componentField }" name="name">
              <FormItem>
                <FormLabel>{{ t('repo.runner.fields.name') }}</FormLabel>
                <FormControl><Input autocomplete="off" v-bind="componentField" /></FormControl>
                <p class="text-xs text-muted-foreground">{{ t('repo.runner.fields.nameHint') }}</p>
                <FormMessage />
              </FormItem>
            </FormField>
            <p v-if="createError" class="text-sm text-destructive">{{ createError }}</p>
          </div>
          <DialogFooter class="mt-6">
            <Button type="button" variant="outline" @click="createOpen = false">{{ t('common.cancel') }}</Button>
            <Button type="submit" :disabled="isSubmitting">{{ isSubmitting ? t('common.submitting') : t('common.submit') }}</Button>
          </DialogFooter>
        </Form>
      </DialogContent>
    </Dialog>

    <Dialog v-model:open="enrollOpen" @update:open="(v) => { if (!v) acknowledge() }">
      <DialogContent :show-close-button="false">
        <DialogHeader>
          <DialogTitle class="flex items-center gap-2">
            <AlertTriangle class="size-5 text-amber-500" />
            {{ t('repo.runner.enrollTitle') }}
          </DialogTitle>
          <DialogDescription>
            {{ t('repo.runner.enrollDescription', { name: enrollRunner?.name || '' }) }}
          </DialogDescription>
        </DialogHeader>

        <div class="space-y-4">
          <div class="rounded-lg border p-3">
            <div class="text-xs uppercase tracking-wide text-muted-foreground">{{ t('repo.runner.enrollToken') }}</div>
            <code class="mt-2 block break-all rounded bg-muted px-3 py-2 font-mono text-sm">{{ enrollToken }}</code>
            <Button class="mt-2 w-full" size="sm" variant="outline" @click="copyEnroll">
              <Copy v-if="!enrollCopied" class="size-4" />
              <Check v-else class="size-4 text-emerald-600" />
              {{ enrollCopied ? t('repo.runner.copied') : t('repo.runner.copyToken') }}
            </Button>
          </div>

          <div class="rounded-lg border p-3">
            <div class="text-xs uppercase tracking-wide text-muted-foreground">{{ t('repo.runner.installOneLiner') }}</div>
            <p class="mt-2 text-xs text-muted-foreground">{{ t('repo.runner.installHint') }}</p>
            <code class="mt-2 block break-all font-mono text-sm">{{ installOneLiner }}</code>
            <Button class="mt-2 w-full" size="sm" variant="outline" @click="copyOneLiner">
              <Copy v-if="!oneLinerCopied" class="size-4" />
              <Check v-else class="size-4 text-emerald-600" />
              {{ oneLinerCopied ? t('repo.runner.copied') : t('repo.runner.copyInstall') }}
            </Button>
          </div>

          <div class="rounded-lg border p-3 text-sm text-muted-foreground">
            <p>{{ t('repo.runner.manualHint') }}</p>
            <code class="mt-2 block break-all font-mono">{{ manualEnroll }}</code>
          </div>
        </div>

        <DialogFooter>
          <Button @click="acknowledge">{{ t('common.acknowledge') }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
