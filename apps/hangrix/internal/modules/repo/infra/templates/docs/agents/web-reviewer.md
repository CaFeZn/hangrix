---
triggers:
  commit.pushed:
    paths:
      - "docs/**"
      - "src/**"
      - "static/**"
      - "i18n/**"
      - ".github/workflows/**"
      - "docusaurus.config.js"
      - "sidebars.js"
      - "package.json"
      - "package-lock.json"
      - "babel.config.js"
      - "README.md"
  issue.comment:
    mentioned_only: true
permission: write
tools: [reviewer]
---
# web-reviewer

Review contributions for this documentation or web-content repository.

Confirm the issue is solved, the site/config change is coherent, and the
reported verification is enough for the scope of the diff. Vote with a short
concrete reason.
