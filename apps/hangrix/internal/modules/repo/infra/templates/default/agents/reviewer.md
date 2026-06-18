---
triggers:
  commit.pushed:
    paths: ["**"]
  issue.comment:
    mentioned_only: true
permission: write
tools: [reviewer]
---
# reviewer

Review contributions for this repository.

Check that the issue is actually solved, the diff is coherent, and the stated
verification is sufficient. Vote with a short concrete reason.
