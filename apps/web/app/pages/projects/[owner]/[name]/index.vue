<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Boxes, FilePlus2, GitBranch, GitPullRequestArrow, Lightbulb, MessageSquare, Plus, Unlink } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import type { Issue, IssueListResp } from '~/types/issue'
import type { Project, ProjectRepo } from '~/types/project'
import type { PublicRepo, RepoListResp } from '~/types/repo'

const { t } = useI18n()
const route = useRoute()
const owner = computed(() => String(route.params.owner))
const name = computed(() => String(route.params.name))

setBreadcrumbs(() => [
  { label: t('project.title'), to: '/projects' },
  { label: `${owner.value}/${name.value}` },
])
useHead({ title: () => `${owner.value}/${name.value} - Hangrix` })

const project = ref<Project | null>(null)
const repos = ref<PublicRepo[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

const repoForm = ref({ owner: '', name: '', purpose: '', role: '' })
const selectedRepoKey = ref('')
const selectedRole = ref('')
const selectedPurpose = ref('')
const issueForm = ref({ owner: '', name: '', issue_number: '', kind: 'implementation', summary: '' })
const selectedIssueRepoKey = ref('')
const selectedIssueNumber = ref('')
const issues = ref<Issue[]>([])
const issuesLoading = ref(false)
const proposalForm = ref({ owner_name: '', repo_name: '', description: '', reason: '', module_boundary: '' })
const goalForm = ref({ title: '', body: '', autoSplit: true })
const selectedGoalRepoKey = ref('')
const actionError = ref<string | null>(null)
const actionSuccess = ref<string | null>(null)
const unlinkingRepoID = ref<number | null>(null)

const CUSTOM_VALUE = '__custom__'
const repoRoleOptions = ['core', 'runtime', 'ui', 'docs', 'tooling', 'examples', 'tests', 'infrastructure']
const repoPurposeOptions = ['coreLibrary', 'app', 'documentation', 'tutorials', 'codegen', 'boardTemplate', 'infrastructure']
const issueKindOptions = ['planning', 'implementation', 'bug', 'docs', 'research', 'maintenance']

const linkedRepoKeys = computed(() => new Set((project.value?.repos ?? []).map((r) => `${r.owner_name}/${r.repo_name}`)))
const selectableRepos = computed(() => repos.value.filter((r) => !linkedRepoKeys.value.has(`${r.owner_name}/${r.name}`)))
const projectRepoOptions = computed(() => project.value?.repos ?? [])
const linkedIssueKeys = computed(() => new Set((project.value?.issue_links ?? []).map((i) => `${i.owner_name}/${i.repo_name}#${i.issue_number}`)))
const selectableIssues = computed(() => {
  const repoKey = selectedIssueRepoKey.value
  return issues.value.filter((issue) => !linkedIssueKeys.value.has(`${repoKey}#${issue.number}`))
})

async function load() {
  loading.value = true
  error.value = null
  try {
    const [projectResp, repoResp] = await Promise.all([
      $fetch<Project>(`/api/projects/${owner.value}/${name.value}`, { credentials: 'include' }),
      $fetch<RepoListResp>('/api/repos/me', { credentials: 'include' }),
    ])
    project.value = projectResp
    repos.value = repoResp.items ?? []
    if (!repoForm.value.owner) repoForm.value.owner = project.value.owner_name
    if (!proposalForm.value.owner_name) proposalForm.value.owner_name = project.value.owner_name
    syncDefaultSelections()
  } catch (e: any) {
    error.value = e?.data?.error ?? t('project.loadFailed')
  } finally {
    loading.value = false
  }
}

function syncDefaultSelections() {
  const firstSelectableRepo = selectableRepos.value[0]
  if (!selectedRepoKey.value && firstSelectableRepo) {
    applyRepoKey(repoKey(firstSelectableRepo))
  }
  const firstProjectRepo = projectRepoOptions.value[0]
  if (!selectedIssueRepoKey.value && firstProjectRepo) {
    applyIssueRepoKey(`${firstProjectRepo.owner_name}/${firstProjectRepo.repo_name}`)
  }
  if (!selectedGoalRepoKey.value && firstProjectRepo) {
    selectedGoalRepoKey.value = `${firstProjectRepo.owner_name}/${firstProjectRepo.repo_name}`
  }
}

function repoKey(repo: PublicRepo) {
  return `${repo.owner_name}/${repo.name}`
}

function applyRepoKey(value: string) {
  selectedRepoKey.value = value
  const repo = repos.value.find((r) => repoKey(r) === value)
  repoForm.value.owner = repo?.owner_name ?? ''
  repoForm.value.name = repo?.name ?? ''
}

function applyIssueRepoKey(value: string) {
  selectedIssueRepoKey.value = value
  const [repoOwner = '', repoName = ''] = value.split('/')
  issueForm.value.owner = repoOwner
  issueForm.value.name = repoName
  issueForm.value.issue_number = ''
  issueForm.value.summary = ''
  selectedIssueNumber.value = ''
  issues.value = []
}

function applyRole(value: string) {
  selectedRole.value = value
  if (value !== CUSTOM_VALUE) repoForm.value.role = value
}

function applyPurpose(value: string) {
  selectedPurpose.value = value
  if (value !== CUSTOM_VALUE) repoForm.value.purpose = t(`project.repoPurpose.${value}`)
}

watch(selectedRepoKey, (value) => {
  if (value) applyRepoKey(value)
})

watch(selectedIssueRepoKey, (value) => {
  if (value) {
    applyIssueRepoKey(value)
    loadIssuesForSelectedRepo()
  }
})

watch(selectedIssueNumber, (value) => {
  issueForm.value.issue_number = value
  const selected = issues.value.find((issue) => String(issue.number) === value)
  if (selected && !issueForm.value.summary) issueForm.value.summary = selected.title
})

async function loadIssuesForSelectedRepo() {
  const [repoOwner = '', repoName = ''] = selectedIssueRepoKey.value.split('/')
  if (!repoOwner || !repoName) return
  issuesLoading.value = true
  try {
    const res = await $fetch<IssueListResp>(`/api/repos/${repoOwner}/${repoName}/issues`, {
      credentials: 'include',
      query: { state: 'all', limit: 100 },
    })
    issues.value = res.items ?? []
  } catch {
    issues.value = []
  } finally {
    issuesLoading.value = false
  }
}

async function addRepo() {
  if (!repoForm.value.owner || !repoForm.value.name) return
  actionError.value = null
  actionSuccess.value = null
  try {
    await $fetch(`/api/projects/${owner.value}/${name.value}/repos`, {
      method: 'POST',
      credentials: 'include',
      body: repoForm.value,
    })
    repoForm.value = { owner: project.value?.owner_name ?? '', name: '', purpose: '', role: '' }
    selectedRepoKey.value = ''
    selectedRole.value = ''
    selectedPurpose.value = ''
    await load()
  } catch (e: any) {
    actionError.value = e?.data?.error ?? t('project.linkRepoFailed')
  }
}

async function unlinkRepo(repo: ProjectRepo) {
  const repoName = `${repo.owner_name}/${repo.repo_name}`
  if (!window.confirm(t('project.unlinkRepoConfirm', { repo: repoName }))) return
  actionError.value = null
  actionSuccess.value = null
  unlinkingRepoID.value = repo.repo_id
  try {
    await $fetch(`/api/projects/${owner.value}/${name.value}/repos/${repo.repo_id}`, {
      method: 'DELETE',
      credentials: 'include',
    })
    if (selectedIssueRepoKey.value === repoName) {
      selectedIssueRepoKey.value = ''
      selectedIssueNumber.value = ''
      issues.value = []
    }
    await load()
  } catch (e: any) {
    actionError.value = e?.data?.error ?? t('project.unlinkRepoFailed')
  } finally {
    unlinkingRepoID.value = null
  }
}

async function linkIssue() {
  const issueNumber = Number(issueForm.value.issue_number)
  if (!issueForm.value.owner || !issueForm.value.name || !issueNumber) return
  actionError.value = null
  actionSuccess.value = null
  try {
    await $fetch(`/api/projects/${owner.value}/${name.value}/issue-links`, {
      method: 'POST',
      credentials: 'include',
      body: {
        owner: issueForm.value.owner,
        name: issueForm.value.name,
        issue_number: issueNumber,
        kind: issueForm.value.kind,
        summary: issueForm.value.summary,
      },
    })
    issueForm.value = { owner: '', name: '', issue_number: '', kind: 'implementation', summary: '' }
    selectedIssueRepoKey.value = ''
    selectedIssueNumber.value = ''
    issues.value = []
    await load()
  } catch (e: any) {
    actionError.value = e?.data?.error ?? t('project.linkIssueFailed')
  }
}

async function createProposal() {
  if (!proposalForm.value.owner_name || !proposalForm.value.repo_name) return
  actionError.value = null
  actionSuccess.value = null
  try {
    await $fetch(`/api/projects/${owner.value}/${name.value}/repo-proposals`, {
      method: 'POST',
      credentials: 'include',
      body: proposalForm.value,
    })
    proposalForm.value = { owner_name: project.value?.owner_name ?? '', repo_name: '', description: '', reason: '', module_boundary: '' }
    await load()
  } catch (e: any) {
    actionError.value = e?.data?.error ?? t('project.proposalFailed')
  }
}

async function createProjectGoal() {
  if (!selectedGoalRepoKey.value || !goalForm.value.title.trim()) return
  actionError.value = null
  actionSuccess.value = null
  const [repoOwner = '', repoName = ''] = selectedGoalRepoKey.value.split('/')
  try {
    const issue = await $fetch<Issue>(`/api/repos/${repoOwner}/${repoName}/issues`, {
      method: 'POST',
      credentials: 'include',
      body: {
        title: goalForm.value.title.trim(),
        body: goalForm.value.body.trim(),
      },
    })
    await $fetch(`/api/projects/${owner.value}/${name.value}/issue-links`, {
      method: 'POST',
      credentials: 'include',
      body: {
        owner: repoOwner,
        name: repoName,
        issue_number: issue.number,
        kind: 'planning',
        summary: goalForm.value.title.trim(),
      },
    })
    if (goalForm.value.autoSplit) {
      await $fetch(`/api/repos/${repoOwner}/${repoName}/issues/${issue.number}/comments`, {
        method: 'POST',
        credentials: 'include',
        body: {
          body: [
            '@agent-planner',
            '',
            `请把这个项目级目标拆分到合适仓库，并把拆出的 issue 链接回项目 \`${owner.value}/${name.value}\`。`,
            '若已有 linked repo 可以承接，请直接在目标 repo 创建 issue；若缺少合适仓库，请创建 repo proposal。',
          ].join('\n'),
        },
      })
    }
    goalForm.value = { title: '', body: '', autoSplit: true }
    actionSuccess.value = t('project.goalCreated', { repo: `${repoOwner}/${repoName}`, number: issue.number })
    await load()
  } catch (e: any) {
    actionError.value = e?.data?.error ?? t('project.createGoalFailed')
  }
}

function visibilityLabel(value: string) {
  return value === 'public' ? t('project.visibilityPublic') : t('project.visibilityPrivate')
}

function issueStateLabel(value: string) {
  switch (value) {
    case 'open':
      return t('project.issueStateOpen')
    case 'closed':
      return t('project.issueStateClosed')
    case 'merged':
      return t('project.issueStateMerged')
    default:
      return value || t('project.unknown')
  }
}

function proposalStatusLabel(value: string) {
  switch (value) {
    case 'pending':
      return t('project.proposalStatusPending')
    case 'approved':
      return t('project.proposalStatusApproved')
    case 'rejected':
      return t('project.proposalStatusRejected')
    case 'provisioned':
      return t('project.proposalStatusProvisioned')
    default:
      return value || t('project.unknown')
  }
}

function issueKindLabel(value: string) {
  return issueKindOptions.includes(value) ? t(`project.issueKind.${value}`) : value
}

function issueOptionLabel(issue: Issue) {
  return `#${issue.number} ${issue.title} · ${issueStateLabel(issue.state)}`
}

onMounted(load)
</script>

<template>
  <div class="space-y-6">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div class="space-y-1">
        <div class="flex flex-wrap items-center gap-2">
          <h1 class="text-2xl font-semibold tracking-tight">
            {{ owner }} / {{ name }}
          </h1>
          <Badge v-if="project" :variant="project.visibility === 'private' ? 'outline' : 'secondary'">
            {{ visibilityLabel(project.visibility) }}
          </Badge>
        </div>
        <p class="text-sm text-muted-foreground">
          {{ project?.description || t('project.detailFallback') }}
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <Button variant="outline" as-child>
          <NuxtLink :to="`/projects/${owner}/${name}/chat`">
            <MessageSquare class="size-4" />
            {{ t('project.chat') }}
          </NuxtLink>
        </Button>
        <Button variant="outline" as-child>
          <NuxtLink to="/projects">
            {{ t('project.title') }}
          </NuxtLink>
        </Button>
      </div>
    </header>

    <p v-if="error" class="text-sm text-destructive">{{ error }}</p>
    <p v-if="loading && !project" class="text-sm text-muted-foreground">{{ t('common.loading') }}</p>

    <template v-if="project">
      <section class="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <Boxes class="size-4" />
              {{ t('project.architecture') }}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <pre class="whitespace-pre-wrap text-sm text-muted-foreground">{{ project.architecture || t('project.noArchitecture') }}</pre>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <GitBranch class="size-4" />
              {{ t('project.moduleBoundaries') }}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <pre class="whitespace-pre-wrap text-sm text-muted-foreground">{{ project.module_boundaries || t('project.noModuleBoundaries') }}</pre>
          </CardContent>
        </Card>
      </section>

      <p v-if="actionError" class="text-sm text-destructive">{{ actionError }}</p>
      <p v-if="actionSuccess" class="text-sm text-emerald-600">{{ actionSuccess }}</p>

      <section class="grid gap-4 xl:grid-cols-3">
        <Card class="xl:col-span-2">
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <FilePlus2 class="size-4" />
              {{ t('project.goalTitle') }}
            </CardTitle>
            <CardDescription>{{ t('project.goalDescription') }}</CardDescription>
          </CardHeader>
          <CardContent>
            <form class="space-y-3" @submit.prevent="createProjectGoal">
              <div class="space-y-1">
                <Label>{{ t('project.goalCoordinatorRepo') }}</Label>
                <Select v-model="selectedGoalRepoKey" :disabled="projectRepoOptions.length === 0">
                  <SelectTrigger>
                    <SelectValue :placeholder="t('project.selectRepo')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem
                      v-for="r in projectRepoOptions"
                      :key="r.id"
                      :value="`${r.owner_name}/${r.repo_name}`"
                    >
                      {{ r.owner_name }} / {{ r.repo_name }}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <p v-if="projectRepoOptions.length === 0" class="text-xs text-muted-foreground">
                  {{ t('project.noGoalRepo') }}
                </p>
              </div>
              <div class="space-y-1">
                <Label>{{ t('issue.fields.title') }}</Label>
                <Input v-model="goalForm.title" :placeholder="t('issue.fields.titlePlaceholder')" />
              </div>
              <div class="space-y-1">
                <Label>{{ t('issue.fields.body') }}</Label>
                <Textarea v-model="goalForm.body" rows="6" :placeholder="t('issue.fields.bodyPlaceholder')" />
              </div>
              <div class="flex items-start gap-3 rounded-md border p-3">
                <Checkbox
                  id="project-goal-auto-split"
                  class="mt-1"
                  :model-value="goalForm.autoSplit"
                  @update:model-value="(v) => goalForm.autoSplit = !!v"
                />
                <div class="space-y-0.5">
                  <Label for="project-goal-auto-split" class="text-sm font-medium">
                    {{ t('project.goalAutoSplit') }}
                  </Label>
                  <p class="text-xs text-muted-foreground">{{ t('project.goalAutoSplitHint') }}</p>
                </div>
              </div>
              <Button type="submit" class="w-full" :disabled="!selectedGoalRepoKey || !goalForm.title.trim()">
                {{ t('project.createGoal') }}
              </Button>
            </form>
          </CardContent>
        </Card>
      </section>

      <section class="grid gap-4 xl:grid-cols-3">
        <Card class="xl:col-span-2">
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <GitBranch class="size-4" />
              {{ t('project.repositories') }}
            </CardTitle>
            <CardDescription>{{ t('project.linkedRepositories', { n: project.repos?.length ?? 0 }) }}</CardDescription>
          </CardHeader>
          <CardContent class="space-y-3">
            <div v-for="r in project.repos" :key="r.id" class="flex items-center justify-between gap-3 rounded-md border p-3">
              <div class="min-w-0">
                <NuxtLink :to="`/${r.owner_name}/${r.repo_name}`" class="truncate font-medium hover:underline">
                  {{ r.owner_name }} / {{ r.repo_name }}
                </NuxtLink>
                <p class="truncate text-xs text-muted-foreground">{{ r.role || t('project.repoFallback') }} · {{ r.purpose || t('project.noPurpose') }}</p>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                class="shrink-0"
                :disabled="unlinkingRepoID === r.repo_id"
                @click="unlinkRepo(r)"
              >
                <Unlink class="size-4" />
                {{ t('project.unlinkRepo') }}
              </Button>
            </div>
            <p v-if="!project.repos?.length" class="text-sm text-muted-foreground">{{ t('project.noRepositories') }}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <Plus class="size-4" />
              {{ t('project.linkRepo') }}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <form class="space-y-3" @submit.prevent="addRepo">
              <div class="space-y-1">
                <Label>{{ t('project.repo') }}</Label>
                <Select v-model="selectedRepoKey" :disabled="selectableRepos.length === 0">
                  <SelectTrigger>
                    <SelectValue :placeholder="t('project.selectRepo')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem v-for="r in selectableRepos" :key="r.id" :value="repoKey(r)">
                      {{ r.owner_name }} / {{ r.name }}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <p v-if="selectableRepos.length === 0" class="text-xs text-muted-foreground">
                  {{ t('project.noSelectableRepositories') }}
                </p>
              </div>
              <div class="space-y-1">
                <Label>{{ t('project.role') }}</Label>
                <Select v-model="selectedRole">
                  <SelectTrigger>
                    <SelectValue :placeholder="t('project.selectRole')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem v-for="role in repoRoleOptions" :key="role" :value="role">
                      {{ t(`project.repoRole.${role}`) }}
                    </SelectItem>
                    <SelectItem :value="CUSTOM_VALUE">{{ t('project.custom') }}</SelectItem>
                  </SelectContent>
                </Select>
                <Input
                  v-if="selectedRole === CUSTOM_VALUE"
                  v-model="repoForm.role"
                  :placeholder="t('project.rolePlaceholder')"
                />
              </div>
              <div class="space-y-1">
                <Label>{{ t('project.purpose') }}</Label>
                <Select v-model="selectedPurpose">
                  <SelectTrigger>
                    <SelectValue :placeholder="t('project.selectPurpose')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem v-for="purpose in repoPurposeOptions" :key="purpose" :value="purpose">
                      {{ t(`project.repoPurpose.${purpose}`) }}
                    </SelectItem>
                    <SelectItem :value="CUSTOM_VALUE">{{ t('project.custom') }}</SelectItem>
                  </SelectContent>
                </Select>
                <Textarea
                  v-if="selectedPurpose === CUSTOM_VALUE"
                  v-model="repoForm.purpose"
                  rows="3"
                  :placeholder="t('project.purposePlaceholder')"
                />
              </div>
              <Button type="submit" class="w-full" :disabled="!repoForm.owner || !repoForm.name">
                {{ t('project.link') }}
              </Button>
            </form>
          </CardContent>
        </Card>
      </section>

      <section class="grid gap-4 xl:grid-cols-3">
        <Card class="xl:col-span-2">
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <GitPullRequestArrow class="size-4" />
              {{ t('project.crossRepoIssues') }}
            </CardTitle>
            <CardDescription>{{ t('project.trackedTasks', { n: project.issue_links?.length ?? 0 }) }}</CardDescription>
          </CardHeader>
          <CardContent class="space-y-3">
            <div v-for="i in project.issue_links" :key="i.id" class="flex items-center justify-between gap-3 rounded-md border p-3">
              <div class="min-w-0">
                <NuxtLink :to="`/${i.owner_name}/${i.repo_name}/issues/${i.issue_number}`" class="truncate font-medium hover:underline">
                  {{ i.owner_name }} / {{ i.repo_name }} #{{ i.issue_number }}
                </NuxtLink>
                <p class="truncate text-xs text-muted-foreground">{{ issueKindLabel(i.kind) }} · {{ i.issue_title }}</p>
              </div>
              <Badge :variant="i.issue_state === 'open' ? 'secondary' : 'outline'">{{ issueStateLabel(i.issue_state) }}</Badge>
            </div>
            <p v-if="!project.issue_links?.length" class="text-sm text-muted-foreground">{{ t('project.noIssueLinks') }}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle class="text-base">{{ t('project.linkIssue') }}</CardTitle>
          </CardHeader>
          <CardContent>
            <form class="space-y-3" @submit.prevent="linkIssue">
              <Select v-model="selectedIssueRepoKey" :disabled="projectRepoOptions.length === 0">
                <SelectTrigger>
                  <SelectValue :placeholder="t('project.selectRepo')" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem
                    v-for="r in projectRepoOptions"
                    :key="r.id"
                    :value="`${r.owner_name}/${r.repo_name}`"
                  >
                    {{ r.owner_name }} / {{ r.repo_name }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <Select
                v-model="selectedIssueNumber"
                :disabled="!selectedIssueRepoKey || issuesLoading || selectableIssues.length === 0"
              >
                <SelectTrigger>
                  <SelectValue
                    :placeholder="issuesLoading ? t('project.loadingIssues') : t('project.selectIssue')"
                  />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem
                    v-for="issue in selectableIssues"
                    :key="issue.id"
                    :value="String(issue.number)"
                  >
                    {{ issueOptionLabel(issue) }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <p
                v-if="selectedIssueRepoKey && !issuesLoading && selectableIssues.length === 0"
                class="text-xs text-muted-foreground"
              >
                {{ t('project.noSelectableIssues') }}
              </p>
              <Select v-model="issueForm.kind">
                <SelectTrigger>
                  <SelectValue :placeholder="t('project.kind')" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="kind in issueKindOptions" :key="kind" :value="kind">
                    {{ t(`project.issueKind.${kind}`) }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <Input v-model="issueForm.summary" :placeholder="t('project.summary')" />
              <Button
                type="submit"
                class="w-full"
                :disabled="!issueForm.owner || !issueForm.name || !issueForm.issue_number"
              >
                {{ t('project.track') }}
              </Button>
            </form>
          </CardContent>
        </Card>
      </section>

      <section class="grid gap-4 xl:grid-cols-3">
        <Card class="xl:col-span-2">
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <Lightbulb class="size-4" />
              {{ t('project.repositoryProposals') }}
            </CardTitle>
            <CardDescription>{{ t('project.proposalCount', { n: project.repo_proposals?.length ?? 0 }) }}</CardDescription>
          </CardHeader>
          <CardContent class="space-y-3">
            <div v-for="p in project.repo_proposals" :key="p.id" class="rounded-md border p-3">
              <div class="flex items-center justify-between gap-3">
                <div class="min-w-0 font-medium">{{ p.owner_name }} / {{ p.repo_name }}</div>
                <Badge variant="outline">{{ proposalStatusLabel(p.status) }}</Badge>
              </div>
              <p class="mt-1 text-sm text-muted-foreground">{{ p.description || p.reason || t('project.noDescription') }}</p>
              <p v-if="p.module_boundary" class="mt-2 whitespace-pre-wrap text-xs text-muted-foreground">{{ p.module_boundary }}</p>
            </div>
            <p v-if="!project.repo_proposals?.length" class="text-sm text-muted-foreground">{{ t('project.noRepoProposals') }}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle class="text-base">{{ t('project.proposeRepo') }}</CardTitle>
          </CardHeader>
          <CardContent>
            <form class="space-y-3" @submit.prevent="createProposal">
              <div class="grid grid-cols-2 gap-2">
                <Input v-model="proposalForm.owner_name" :placeholder="t('project.owner')" />
                <Input v-model="proposalForm.repo_name" :placeholder="t('project.repo')" />
              </div>
              <Input v-model="proposalForm.description" :placeholder="t('project.description')" />
              <Textarea v-model="proposalForm.reason" rows="3" :placeholder="t('project.reason')" />
              <Textarea v-model="proposalForm.module_boundary" rows="4" :placeholder="t('project.moduleBoundary')" />
              <Button type="submit" class="w-full">{{ t('project.submitProposal') }}</Button>
            </form>
          </CardContent>
        </Card>
      </section>
    </template>
  </div>
</template>
