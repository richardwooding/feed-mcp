# Edge cases

Cases the audit and apply scripts intentionally hand back to a human reviewer rather than auto-fix.

## Contents

- Composite actions
- Docker actions
- Local action references
- Reusable workflows
- Archived or abandoned actions
- The two escape flags
- Marketplace actions hosted under user accounts
- Monorepos with sub-path actions
- Self-hosted runners

## Composite actions

A composite action declares `runs.using: composite` and orchestrates other steps; it does not run on Node itself. The audit classifies these as `composite` and recommends the latest release tag (if any) but does not infer a Node version. The Node concern lives one level deeper: the composite's own `steps[].uses:` references are what eventually run on Node 20 or Node 24.

To audit a composite's internals, run the audit against a checkout of the composite's repo. The script descends into nested `action.yml` files automatically — what it does not do is fetch the composite's source over the network and recurse into it. That is intentional: latency would balloon, and a single composite can transitively reference dozens of upstream actions.

## Docker actions

`runs.using: docker` is unaffected — these execute the container's entrypoint, not the runner's bundled Node. The audit classifies them `docker` and skips them. Nothing further is needed.

## Local action references

A workflow line like `uses: ./local-action` resolves to an `action.yml` inside the same repo. The audit records the workflow line as `local-action` (so the reviewer sees it) and separately classifies the local `action.yml` itself as `local-node20`, `local-node24`, `composite`, or `docker`. The fix lands on the `action.yml`, not the workflow line.

## Reusable workflows

A line like `uses: org/repo/.github/workflows/build.yml@ref` consumes a *workflow*, not an action. Workflow YAML has no `runs.using`; the deprecation does not apply at this layer. The audit classifies these as `reusable-workflow` and skips them. Update the actions *inside* the reusable workflow at its source repo, not at the consumer.

## Archived or abandoned actions

If `gh api` returns 404 or the latest release still declares `node20`, the audit classifies the action as `unresolved` or `node20-stuck`. There is no auto-fix. Choices:

1. Find a maintained alternative.
2. Fork the action, change `runs.using: node20` to `node24` in the fork, smoke-test under `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true`, and pin to the fork's SHA.
3. Strip the action from the workflow if its work can be replaced by a `run:` step.

Document the choice in the PR — abandoned-action escapes have a habit of becoming permanent.

## The two escape flags

`FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true` — set in a workflow's `env:` block. Forces every JavaScript action in that run to execute on Node 24, regardless of its declared `runs.using`. Use to smoke-test a workflow before bumping refs. Safe to leave on indefinitely; the flag becomes a no-op once Node 24 is the default.

`ACTIONS_ALLOW_USE_UNSECURE_NODE_VERSION=true` — set on the runner (env, not workflow). Allows a Node 20 action to keep running after the runner default flips. Treat as a temporary unblock, not a fix. Stops working entirely once Node 20 is removed from the runner image. Skill scripts never write either flag — surfacing them is a human decision.

## Marketplace actions hosted under user accounts

Actions live at any `<owner>/<repo>` slug — they are not all under organisations like `actions/`. The audit treats every external slug identically: hit `gh api repos/<owner>/<repo>/contents/action.yml`, read `runs.using`, fetch `releases/latest`, classify. No special handling is needed.

## Monorepos with sub-path actions

Some repos publish multiple actions, each at a sub-path: `uses: org/repo/path/to/action@v3`. The audit splits the slug correctly (`slug=org/repo`, `sub_path=path/to/action`) and fetches `<sub_path>/action.yml`. Apply preserves the sub-path on rewrite.

## Self-hosted runners

The deprecation affects the Node binary that ships *with* the runner, not the OS-level Node. Self-hosted runners follow the same timeline as GitHub-hosted ones once they upgrade `actions/runner` past v2.328.0. Two extra concerns for self-hosted:

- **macOS 13.4 or earlier** — Node 24 will not run; upgrade the host OS or the runner falls back to (and eventually loses) Node 20.
- **ARM32** — Node 24 has no ARM32 build; these runners cannot execute Node 24 actions at all.

The audit script does not detect runner OS or architecture. Read `runs-on:` in the workflow to spot self-hosted labels (`self-hosted`, custom labels) and verify the host fleet manually.
