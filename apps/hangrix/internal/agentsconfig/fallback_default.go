package agentsconfig

// Built-in fallback used when a repository has no committed `.hangrix`
// directory yet. This keeps new repos runnable without silently writing config
// files into the repo; once a user bootstraps or commits their own `.hangrix`,
// that explicit config takes precedence.

const fallbackDefaultAgentsYAML = `version: 1

container:
  image: node:22-bookworm
  env:
    NODE_ENV: development
  volumes:
    - { name: npm-cache, mount: /root/.npm }

llm:
  model: worker
  reasoning_effort: medium
  max_output_tokens: 8192

tools:
  all: ["*"]
  worker:
    - issue_read
    - issue_read_by_number
    - issue_comment_read
    - issue_comment
    - issue_comment_cross
    - issue_create
    - issue_edit
    - issue_mergeable
    - issue_todo_list
    - issue_todo_update
    - issue_attachment_upload
    - issue_children
    - issue_checks
    - contribution_list
    - contribution_read
    - contribution_set_meta
    - contribution_close
    - roster_list
  reviewer:
    - issue_read
    - issue_read_by_number
    - issue_comment_read
    - issue_comment
    - issue_review_vote
    - issue_mergeable
    - issue_children
    - issue_checks
    - contribution_list
    - contribution_read
    - roster_list
  fast:
    - issue_read
    - issue_read_by_number
    - issue_comment_read
    - issue_comment
    - issue_comment_cross
    - issue_create
    - issue_edit
    - issue_mergeable
    - issue_todo_list
    - issue_todo_update
    - issue_attachment_upload
    - issue_children
    - issue_checks
    - contribution_list
    - contribution_read
    - contribution_set_meta
    - contribution_close
    - roster_list

reviewers:
  rules:
    - paths: ["**"]
      reviewers: [reviewer]
  fallback: [reviewer]
`

const fallbackDefaultMaintainerMD = `---
triggers:
  issue.opened: {}
  issue.comment: {}
  review_vote.posted: {}
permission: write
tools: [all]
---
# maintainer

You are the repository maintainer.

Route narrow repetitive work to @agent-fast-worker, broader implementation to
@agent-worker, keep todos current, and move approved contributions through
review and apply. Do not implement feature code yourself.
`

const fallbackDefaultWorkerMD = `---
triggers:
  issue.comment:
    mentioned_only: true
permission: write
tools: [worker]
---
# worker

Implement the requested change in this repository.

Read the issue, inspect the relevant files, make the smallest coherent change,
and report what you changed and how you verified it.
`

const fallbackDefaultReviewerMD = `---
triggers:
  commit.pushed:
    paths: ["**"]
  issue.comment:
    mentioned_only: true
permission: write
tools: [reviewer]
llm:
  model: reviewer
---
# reviewer

Review contributions for this repository.

Check that the issue is actually solved, the diff is coherent, and the stated
verification is sufficient. Vote with a short concrete reason.
`

const fallbackDefaultFastWorkerMD = `---
triggers:
  issue.comment:
    mentioned_only: true
permission: write
tools: [fast]
llm:
  model: fast
  reasoning_effort: low
  max_output_tokens: 8192
---
# fast-worker

Implement narrow, low-risk tasks quickly. Wake only on @agent-fast-worker
mention.

Use this role for small mechanical edits, copy tweaks, obvious config fixes,
and straightforward test updates. If the scope expands, hand the task back to
@agent-worker.
`

func fallbackDefaultFiles() map[string][]byte {
	return map[string][]byte{
		HostConfigPath:                []byte(fallbackDefaultAgentsYAML),
		AgentsDir + "/maintainer.md":  []byte(fallbackDefaultMaintainerMD),
		AgentsDir + "/worker.md":      []byte(fallbackDefaultWorkerMD),
		AgentsDir + "/reviewer.md":    []byte(fallbackDefaultReviewerMD),
		AgentsDir + "/fast-worker.md": []byte(fallbackDefaultFastWorkerMD),
	}
}
