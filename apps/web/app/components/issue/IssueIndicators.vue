<script setup lang="ts">
import { AtSign, LoaderCircle, MessageCircleQuestion } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import type { IssueIndicators } from '~/types/issue'

defineProps<{
  indicators?: IssueIndicators
  questionnaireTo?: string
}>()

const { t } = useI18n()
</script>

<template>
  <span v-if="indicators?.pending_questionnaire || indicators?.agent_mention || indicators?.queued_agent_run" class="flex items-center gap-1">
    <NuxtLink
      v-if="indicators?.pending_questionnaire && questionnaireTo"
      :to="questionnaireTo"
      class="inline-flex"
    >
      <Badge
        class="gap-1 bg-amber-500/15 text-amber-700 transition-colors hover:bg-amber-500/25 dark:text-amber-300"
        variant="secondary"
      >
        <MessageCircleQuestion class="size-3" />
        {{ t('issue.indicators.pendingQuestionnaire') }}
      </Badge>
    </NuxtLink>
    <Badge
      v-else-if="indicators?.pending_questionnaire"
      class="gap-1 bg-amber-500/15 text-amber-700 dark:text-amber-300"
      variant="secondary"
    >
      <MessageCircleQuestion class="size-3" />
      {{ t('issue.indicators.pendingQuestionnaire') }}
    </Badge>
    <Badge v-if="indicators?.agent_mention"
           class="gap-1 bg-sky-500/15 text-sky-700 dark:text-sky-300"
           variant="secondary">
      <AtSign class="size-3" />
      {{ t('issue.indicators.mentioned') }}
    </Badge>
    <Badge
      v-if="indicators?.queued_agent_run"
      class="gap-1 bg-emerald-500/15 text-emerald-700 dark:text-emerald-300"
      variant="secondary"
    >
      <LoaderCircle class="size-3 animate-spin" />
      {{ t('issue.indicators.queuedAgentRun') }}
    </Badge>
  </span>
</template>
