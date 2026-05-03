# Workflow author guide

Use this when the repo *consumes* GitHub Actions but does not publish any. The job is to bump `uses:` references in `.github/workflows/*.yml` so every action lands on a release whose `action.yml` declares Node 24.

## Contents

- The mental model
- Common-action migration table
- SHA-pinned actions
- Dependabot keeps it evergreen
- Verifying with the early-test flag
- When the upstream has no Node 24 release yet

## The mental model

A workflow line like `uses: actions/checkout@v4` resolves at runtime to the `action.yml` published at the `v4` tag. That file declares `runs.using: node20` today. The fix is to pin to a release whose `action.yml` declares `runs.using: node24`. The audit script does this lookup automatically. The apply script rewrites the `@<ref>` portion in place.

There is nothing to change in `runs:` blocks of workflow files (workflow YAML has no `runs.using`). The only edit on the consumer side is the `@<ref>` after each action slug.

## Common-action migration table

These are the action families the audit will most often touch. Don't bake the right-hand column into anything — let the audit script discover the current Node 24 release at run time. The list is here only so a reviewer recognises which actions matter.

| Action family                              | Today's typical Node 20 ref | Where Node 24 lands     |
|--------------------------------------------|-----------------------------|-------------------------|
| `actions/checkout`                         | `@v4`                       | `@v5` major or later    |
| `actions/setup-node`                       | `@v4`                       | next major              |
| `actions/setup-go`                         | `@v5`                       | next major              |
| `actions/setup-python`                     | `@v5`                       | next major              |
| `actions/setup-java`                       | `@v4`                       | next major              |
| `actions/cache`                            | `@v4`                       | next major              |
| `actions/upload-artifact`                  | `@v4`                       | next major              |
| `actions/download-artifact`                | `@v4`                       | next major              |
| `actions/configure-pages`                  | `@v5`                       | next major              |
| `actions/upload-pages-artifact`            | `@v3`                       | next major              |
| `actions/deploy-pages`                     | `@v4`                       | next major              |
| `actions/labeler`                          | `@v5`                       | next major              |
| `goreleaser/goreleaser-action`             | `@v6`                       | next major              |
| `golangci/golangci-lint-action`            | `@v7` / `@v8`               | already shipping Node 24 in newer majors |
| `docker/setup-buildx-action`               | `@v3`                       | next major              |
| `docker/login-action`                      | `@v3`                       | next major              |
| `docker/build-push-action`                 | `@v6`                       | next major              |

A new major often crosses other breaking changes (changed default inputs, dropped flags). Before approving a bump that crosses a major, read the upstream release notes at `https://github.com/<slug>/releases`. The audit's `notes` column links to those when a major-version bump is detected.

## SHA-pinned actions

Many security-conscious repos pin to a 40-character commit SHA, not a tag, e.g.:

```yaml
uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332  # v4.1.7
```

The audit detects SHA pins (`pin_style: sha`) and the apply script preserves the style: it resolves the recommended tag back to a SHA via `gh api repos/<slug>/commits/<tag>` and writes the SHA, leaving the trailing `# vX.Y.Z` comment intact. Update the comment manually in the same edit (the apply script does not touch comments — it only edits the `@<ref>` portion of the line).

If a SHA pin has no trailing comment, add one in the same PR. A bare 40-character SHA is unreviewable in a diff.

## Dependabot keeps it evergreen

Once the migration is done, add or update `.github/dependabot.yml` to keep actions current:

```yaml
version: 2
updates:
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
    groups:
      actions:
        patterns: ["*"]
```

This is a one-time setup that means future Node bumps land as PRs without another audit run.

## Verifying with the early-test flag

Set `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true` in a workflow's `env:` block to force every JavaScript action in that run to execute on Node 24, even if its `action.yml` still declares `node20`. Use this to smoke-test that workflows still pass before swapping refs:

```yaml
env:
  FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: "true"
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: echo "running on Node 24 even though checkout@v4 declares node20"
```

If the workflow passes with this flag, the bump to a Node 24 release is mechanical. If it fails, the failure is the action's, not the workflow's — file an upstream issue.

## When the upstream has no Node 24 release yet

The audit classifies these as `node20-stuck`. Options, ranked:

1. **Wait.** Most popular actions will ship a Node 24 release before the runner default flips. Watch the upstream repo or rely on Dependabot.
2. **Fork and patch** if the action is critical and unmaintained. Change `runs.using: node20` to `node24` in the fork, smoke-test with `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true`, then pin the workflow to the fork's SHA.
3. **Replace.** Many actions have actively-maintained alternatives. The audit's `notes` column does not suggest replacements — that judgment is left to the reviewer.
4. **Last resort:** set `ACTIONS_ALLOW_USE_UNSECURE_NODE_VERSION=true` on the runner to keep executing the action on Node 20 past the cutover. This works only until Node 20 is fully removed and is not a long-term answer.
