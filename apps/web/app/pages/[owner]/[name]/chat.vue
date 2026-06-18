<script setup lang="ts">
import { computed, ref } from 'vue'
import { FilePlus2, MessageSquare, Send } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import type { Issue } from '~/types/issue'

definePageMeta({ layout: 'repo' })

type ChatRole = 'assistant' | 'user'
interface ChatMessage {
  id: number
  role: ChatRole
  body: string
}

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const owner = computed(() => String(route.params.owner ?? ''))
const name = computed(() => String(route.params.name ?? ''))
const scopeLabel = computed(() => `${owner.value}/${name.value}`)

setBreadcrumbs(() => {
  const base = `/${owner.value}/${name.value}`
  return [
    { label: owner.value, to: base },
    { label: name.value, to: base },
    { label: t('chat.repo.title') },
  ]
})
useHead({ title: () => `${t('chat.repo.title')} · ${scopeLabel.value} - ${t('app.name')}` })

let nextID = 1
const messages = ref<ChatMessage[]>([
  { id: nextID++, role: 'assistant', body: t('chat.repo.seed') },
])
const input = ref('')
const issueTitle = ref('')
const titleTouched = ref(false)
const submitting = ref(false)
const error = ref<string | null>(null)

const userMessages = computed(() => messages.value.filter((m) => m.role === 'user'))
const canCreate = computed(() => userMessages.value.length > 0 && issueTitle.value.trim().length > 0 && !submitting.value)

function titleFrom(body: string) {
  const first = body.split(/\r?\n/).map((line) => line.trim()).find(Boolean) ?? t('chat.fallbackTitle')
  return first.length > 120 ? `${first.slice(0, 117)}...` : first
}

function sendMessage() {
  const body = input.value.trim()
  if (!body) return
  messages.value.push({ id: nextID++, role: 'user', body })
  messages.value.push({ id: nextID++, role: 'assistant', body: t('chat.recorded') })
  input.value = ''
  if (!titleTouched.value || !issueTitle.value.trim()) {
    issueTitle.value = titleFrom(body)
    titleTouched.value = false
  }
}

function roleLabel(role: ChatRole) {
  return role === 'user' ? t('chat.user') : t('chat.assistant')
}

function buildIssueBody() {
  const transcript = messages.value
    .map((m) => `**${roleLabel(m.role)}**\n\n${m.body.trim()}`)
    .join('\n\n---\n\n')
  return [
    `## ${t('chat.issueBody.context')}`,
    '',
    `- ${t('chat.issueBody.scope')}: ${scopeLabel.value}`,
    `- ${t('chat.issueBody.source')}: ${t('chat.repo.title')}`,
    '',
    `## ${t('chat.issueBody.transcript')}`,
    '',
    transcript,
  ].join('\n')
}

const issueBodyPreview = computed(() => buildIssueBody())

async function createIssue() {
  if (!canCreate.value) return
  submitting.value = true
  error.value = null
  try {
    const iss = await $fetch<Issue>(`/api/repos/${owner.value}/${name.value}/issues`, {
      method: 'POST',
      credentials: 'include',
      body: {
        title: issueTitle.value.trim(),
        body: buildIssueBody(),
      },
    })
    router.push(`/${owner.value}/${name.value}/issues/${iss.number}`)
  } catch (e: any) {
    error.value = e?.data?.error ?? t('chat.createIssueFailed')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="space-y-6">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div class="space-y-1">
        <h1 class="text-2xl font-semibold tracking-tight">
          {{ t('chat.repo.title') }}
        </h1>
        <p class="text-sm text-muted-foreground">
          {{ scopeLabel }}
        </p>
      </div>
      <Button variant="outline" as-child>
        <NuxtLink :to="`/${owner}/${name}/issues/new`">
          <FilePlus2 class="size-4" />
          {{ t('issue.new') }}
        </NuxtLink>
      </Button>
    </header>

    <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
      <Card class="min-h-[560px]">
        <CardHeader>
          <CardTitle class="flex items-center gap-2 text-base">
            <MessageSquare class="size-4" />
            {{ t('chat.conversation') }}
          </CardTitle>
          <CardDescription>{{ t('chat.repo.description') }}</CardDescription>
        </CardHeader>
        <CardContent class="flex min-h-[460px] flex-col gap-4">
          <div class="flex-1 space-y-3 overflow-y-auto rounded-md border bg-muted/20 p-3">
            <div
              v-for="message in messages"
              :key="message.id"
              class="flex"
              :class="message.role === 'user' ? 'justify-end' : 'justify-start'"
            >
              <div
                class="max-w-[86%] rounded-md border px-3 py-2 text-sm leading-relaxed"
                :class="message.role === 'user' ? 'bg-primary text-primary-foreground' : 'bg-background'"
              >
                <div class="mb-1 text-xs opacity-70">{{ roleLabel(message.role) }}</div>
                <div class="whitespace-pre-wrap break-words">{{ message.body }}</div>
              </div>
            </div>
          </div>

          <div class="flex gap-2">
            <Textarea
              v-model="input"
              rows="3"
              class="min-h-20"
              :placeholder="t('chat.inputPlaceholder')"
              @keydown.ctrl.enter.prevent="sendMessage"
            />
            <Button type="button" class="self-end" :disabled="!input.trim()" @click="sendMessage">
              <Send class="size-4" />
              {{ t('chat.send') }}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle class="text-base">{{ t('chat.issueDraft') }}</CardTitle>
          <CardDescription>{{ t('chat.issueDraftDescription') }}</CardDescription>
        </CardHeader>
        <CardContent class="space-y-4">
          <div class="space-y-2">
            <Label>{{ t('issue.fields.title') }}</Label>
            <Input
              v-model="issueTitle"
              :placeholder="t('issue.fields.titlePlaceholder')"
              @input="titleTouched = true"
            />
          </div>

          <div class="space-y-2">
            <Label>{{ t('issue.fields.body') }}</Label>
            <Textarea
              :model-value="issueBodyPreview"
              readonly
              rows="18"
              class="min-h-80 text-xs leading-relaxed"
            />
          </div>

          <p v-if="error" class="text-sm text-destructive">{{ error }}</p>
          <Button class="w-full" :disabled="!canCreate" @click="createIssue">
            <FilePlus2 class="size-4" />
            {{ submitting ? t('common.submitting') : t('chat.createIssue') }}
          </Button>
        </CardContent>
      </Card>
    </div>
  </div>
</template>
