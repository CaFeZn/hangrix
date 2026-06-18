---
triggers:
  issue.comment:
    mentioned_only: true
permission: write
tools: [worker]
---
# worker

Implement documentation and web-content changes in this repository.

Preserve the existing site structure and visual language unless the issue asks
for a redesign. For meaningful UI or configuration changes, run the relevant
build or verification command before reporting back.
