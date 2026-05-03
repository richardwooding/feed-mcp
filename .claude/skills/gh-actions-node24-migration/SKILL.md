---
name: gh-actions-node24-migration
description: AuditPs GitHub Actions workflows and action.yml metadata for the Node 20 to Node 24 runner deprecation, then bumps `uses:` action versions and rewrites `runs.using: node20` to `node24`. Use when reviewing or upgrading any repo's `.github/workflows/*.yml` or `action.yml`/`action.yaml`, or when the prompt mentions Node 20 deprecation, FORCE_JAVASCRIPT_ACTIONS_TO_NODE24, or ACTIONS_ALLOW_USE_UNSECURE_NODE_VERSION.
---

# GitHub Actions Node 24 Migration

GitHub is retiring the Node 20 runtime on Actions runners. The runner default flips from Node 20 to Node 24, then Node 20 is removed entirely. Two audiences must act:

- **Workflow authors** — bump `uses:` references to action versions whose published `action.yml` declares Node 24.
- **Action authors** — change `runs.using: node20` to `node24` in their action's metadata file.

This skill drives that migration deterministically: a single audit script classifies every action reference in a repo, and an apply script rewrites the safe ones in place.

## Quick start

```sh
# 1. Audit (read-only). Writes audit.json next to the cwd and prints a markdown table.
python scripts/audit.py <repo-root>

# 2. Review the table. Decide which findings to take.

# 3. Dry-run the rewrite (default — prints unified diffs, writes nothing).
python scripts/apply_fix.py audit.json

# 4. Apply for real once the diffs look right.
python scripts/apply_fix.py audit.json --apply
```

`gh` (GitHub CLI) must be installed and authenticated — the audit calls `gh api` to read each action's `action.yml` at its pinned ref and to discover the latest Node 24 release.

## What gets audited

- `**/.github/workflows/*.yml` and `*.yaml` — every `uses:` line (job steps and reusable workflows).
- Top-level `action.yml`/`action.yaml` and any nested `**/action.yml` — the `runs.using` field.
- Composite actions — recursed into `runs.steps[].uses`.

Actions are classified into one of these states:

| State            | Meaning                                                          | Auto-fixable     |
|------------------|------------------------------------------------------------------|------------------|
| `node24`         | Already on Node 24.                                              | —                |
| `node20-fixable` | On Node 20; a Node 24 release exists upstream.                   | yes              |
| `node20-stuck`   | On Node 20; no Node 24 release published yet.                    | no — flag only   |
| `local-node20`   | Local `action.yml` declares `runs.using: node20`.                | yes (rewrite)    |
| `composite`      | Composite action — judged by its inner `uses:` references.       | recursed         |
| `docker`         | Docker-based action; not affected by the Node deprecation.       | skip             |
| `unresolved`     | `action.yml` not reachable (private, archived, deleted).         | no — flag only   |

For non-obvious cases — SHA pins, archived actions, the `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24` and `ACTIONS_ALLOW_USE_UNSECURE_NODE_VERSION` escape flags, reusable-workflow refs — see [references/edge-cases.md](references/edge-cases.md).

## Scripts

- **Run** `python scripts/audit.py <repo-root> [--out audit.json]` — walks the repo, calls `gh api` per action, prints a markdown table to stdout, and writes structured findings to `audit.json` (default path: `./audit.json`). Exits non-zero if any required tool is missing.
- **Run** `python scripts/apply_fix.py <audit.json> [--apply]` — without `--apply`, prints unified diffs only. With `--apply`, rewrites files in place: workflow `uses:` lines are bumped to the recommended ref preserving the original pin style (tag vs full SHA), and local `action.yml` `runs.using: node20` becomes `node24`. Exits non-zero if any finding was skipped (so CI can gate on a clean run).
- **See** `scripts/audit.py` only when an unusual workflow shape needs custom classification — most users just run it.

## References

- [references/workflow-author.md](references/workflow-author.md) — perspective for repos that consume actions: bumping `uses:` versions, handling SHA pins, common-action migration table, dependabot config.
- [references/action-author.md](references/action-author.md) — perspective for repos that publish actions: editing `runs.using`, Node 24 platform/runtime caveats (macOS, ARM32), release/test checklist.
- [references/edge-cases.md](references/edge-cases.md) — composite actions, Docker actions, local actions, reusable workflows, archived/abandoned actions, the early-test and stay-on-Node-20 escape flags.

## Conventions

- Audit before fix; `--apply` is opt-in. Never rewrite without first showing diffs.
- Preserve the existing pin style: tag-major (`@v4`) stays tag-major, full SHA stays full SHA, branch stays branch.
- Never bump a major version silently — when the recommended Node 24 release crosses a major, surface the upstream release notes URL in the audit table so a human can review breaking changes before approving.
- Don't touch third-party actions classed `node20-stuck`. Flag them and stop. The fix is upstream, not in the consuming repo.
- Don't autocommit, push, or open PRs. Edits land locally for human review.
- Skip Docker (`runs.using: docker`) actions — they don't run the runner-internal Node.
- Treat `setup-node`'s `node-version` input as out of scope — that's the workflow's chosen Node, not the runner's.

## Evaluation scenarios

1. **Workflow audit, fully on Node 20.** Prompt: "Check if our workflows need updating for the Node 20 deprecation." Expected: skill triggers, runs `audit.py .`, reports a table classifying every `uses:` line, recommends Node 24 versions where they exist upstream.
2. **Local action.yml on `node20`.** Prompt: "Migrate this action.yml off Node 20." Expected: skill detects the local `runs.using: node20`, dry-runs `apply_fix.py` showing the `node24` rewrite, applies after confirmation.
3. **Decline irrelevant prompt.** Prompt: "Bump the Node version in `setup-node` to 22 in our build workflow." Expected: skill recognises this as a workflow-Node concern unrelated to the runner-internal Node deprecation, and either declines or explicitly notes the distinction before acting.
