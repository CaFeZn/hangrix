<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { FilePlus2, MessageSquare, Send } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import type { Issue } from '~/types/issue'
import type { Project } from '~/types/project'

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
const projectLabel = computed(() => `${owner.value}/${name.value}`)

setBreadcrumbs(() => [
  { label: t('project.title'), to: '/projects' },
  { label: projectLabel.value, to: `/projects/${owner.value}/${name.value}` },
  { label: t('chat.project.title') },
])
useHead({ title: () => `${t('chat.project.title')} · ${projectLabel.value} - ${t('app.name')}` })

let nextID = 1
const messages = ref<ChatMessage[]>([
  { id: nextID++, role: 'assistant', body: t('chat.project.seed') },
])
const input = ref('')
const issueTitle = ref('')
const titleTouched = ref(false)
const selectedRepo = ref('')
const project = ref<Project | null>(null)
const loading = ref(false)
const submitting = ref(false)
const error = ref<string | null>(null)

const repos = computed(() => project.value?.repos ?? [])
const userMessages = computed(() => messages.value.filter((m) => m.role === 'user'))
const selectedRepoParts = computed(() => {
  const [repoOwner = '', repoName = ''] = selectedRepo.value.split('/')
  return { repoOwner, repoName }
})
const canCreate = computed(() => {
  return userMessages.value.length > 0
    && issueTitle.value.trim().length > 0
    && !!selectedRepoParts.value.repoOwner
    && !!selectedRepoParts.value.repoName
    && !submitting.value
})

watch(repos, (items) => {
  const first = items[0]
  if (!selectedRepo.value && first) {
    selectedRepo.value = `${first.owner_name}/${first.repo_name}`
  }
})

async function loadProject() {
  loading.value = true
  error.value = null
  try {
    project.value = await $fetch<Project>(`/api/projects/${owner.value}/${name.value}`, { credentials: 'include' })
  } catch (e: any) {
    error.value = e?.data?.error ?? t('project.loadFailed')
  } finally {
    loading.value = false
  }
}

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
    `- ${t('chat.issueBody.scope')}: ${projectLabel.value}`,
    `- ${t('chat.issueBody.targetRepo')}: ${selectedRepo.value || t('chat.project.noTarget')}`,
    `- ${t('chat.issueBody.source')}: ${t('chat.project.title')}`,
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
  const { repoOwner, repoName } = selectedRepoParts.value
  try {
    const iss = await $fetch<Issue>(`/api/repos/${repoOwner}/${repoName}/issues`, {
      method: 'POST',
      credentials: 'include',
      body: {
        title: issueTitle.value.trim(),
        body: buildIssueBody(),
      },
    })
    await $fetch(`/api/projects/${owner.value}/${name.value}/issue-links`, {
      method: 'POST',
      credentials: 'include',
      body: {
        owner: repoOwner,
        name: repoName,
        issue_number: iss.number,
        kind: 'implementation',
        summary: issueTitle.value.trim(),
      },
    })
    router.push(`/${repoOwner}/${repoName}/issues/${iss.number}`)
  } catch (e: any) {
    error.value = e?.data?.error ?? t('chat.createIssueFailed')
  } finally {
    submitting.value = false
  }
}

onMounted(loadProject)
</script>

<template>
  <div class="space-y-6">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div class="space-y-1">
        <div class="flex flex-wrap items-center gap-2">
          <h1 class="text-2xl font-semibold tracking-tight">
            {{ t('chat.project.title') }}
          </h1>
          <Badge v-if="project" variant="secondary">{{ projectLabel }}</Badge>
        </div>
        <p class="text-sm text-muted-foreground">
          {{ project?.description || t('project.detailFallback') }}
        </p>
      </div>
      <Button variant="outline" as-child>
        <NuxtLink :to="`/projects/${owner}/${name}`">
          {{ t('project.title') }}
        </NuxtLink>
      </Button>
    </header>

    <p v-if="loading" class="text-sm text-muted-foreground">{{ t('common.loading') }}</p>

    <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
      <Card class="min-h-[560px]">
        <CardHeader>
          <CardTitle class="flex items-center gap-2 text-base">
            <MessageSquare class="size-4" />
            {{ t('chat.conversation') }}
          </CardTitle>
          <CardDescription>{{ t('chat.project.description') }}</CardDescription>
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
          <CardDescription>{{ t('chat.project.issueDraftDescription') }}</CardDescription>
        </CardHeader>
        <CardContent class="space-y-4">
          <div class="space-y-2">
            <Label>{{ t('chat.project.targetRepo') }}</Label>
            <Select v-model="selectedRepo" :disabled="repos.length === 0">
              <SelectTrigger>
                <SelectValue :placeholder="t('chat.project.targetRepoPlaceholder')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  v-for="repo in repos"
                  :key="repo.id"
                  :value="`${repo.owner_name}/${repo.repo_name}`"
                >
                  {{ repo.owner_name }} / {{ repo.repo_name }}
                </SelectItem>
              </SelectContent>
            </Select>
            <p v-if="repos.length === 0" class="text-xs text-muted-foreground">
              {{ t('chat.project.noRepos') }}
            </p>
          </div>

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
              rows="16"
              class="min-h-72 text-xs leading-relaxed"
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
