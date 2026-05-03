# Action author guide

Use this when the repo *publishes* a JavaScript GitHub Action (i.e., it has an `action.yml` or `action.yaml` whose `runs.using` is `node20`). The job is to ship a release whose `action.yml` declares `node24`, without breaking consumers.

## Contents

- The single required edit
- Verifying the action runs on Node 24
- Platform and runtime caveats with Node 24
- Release checklist
- What to do if dependencies block the bump

## The single required edit

```diff
 runs:
-  using: node20
+  using: node24
   main: dist/index.js
```

That's it. The runner reads `using:` and selects the corresponding bundled Node binary. `apply_fix.py` performs this exact rewrite for any local `action.yml` the audit classifies as `local-node20`.

If the action also has a `pre:` or `post:` script, those run under the same Node version — no separate change needed.

## Verifying the action runs on Node 24

Two checks before tagging the release:

1. **Local sanity.** Run the action's bundled script under a Node 24 binary:

   ```sh
   nvm install 24 && nvm use 24
   node dist/index.js
   ```

   The action should execute its happy path without import or syntax errors. Most failures here are about dropped Node APIs — the migration guide at `https://nodejs.org/en/blog/announcements/v24-release-announce` lists what changed since Node 20.

2. **End-to-end on a runner.** Push a branch with `runs.using: node24` and a workflow that consumes the action via `uses: <owner>/<repo>@<branch>`. Watch the run log. Look for warnings about deprecated APIs in addition to outright failures.

## Platform and runtime caveats with Node 24

Node 24 is stricter about platforms than Node 20. From the GitHub announcement:

- **macOS 13.4 and earlier** — not supported. Self-hosted runners on older macOS must upgrade.
- **ARM32** — dropped. Self-hosted runners on 32-bit ARM cannot run Node 24 actions. (GitHub-hosted runners are unaffected.)

Action code itself rarely needs to care about either, but call this out in the release notes so consumers running self-hosted on those platforms know.

Other behaviour changes worth scanning the action source for:

- `node:fs` — promise-based methods are now the documented default; sync APIs in hot paths emit warnings.
- `node:url` — `url.parse` is fully deprecated; use `new URL(...)`.
- `process.binding(...)` — removed.
- `Buffer()` constructor — emits an error; use `Buffer.alloc` / `Buffer.from`.

If `package.json` has an `engines.node` field, bump it to `>=24` in the same release.

## Release checklist

1. Edit `action.yml`: `runs.using: node20` → `node24`.
2. Bump `engines.node` in `package.json` (if present).
3. Rebuild the bundled `dist/index.js` (e.g. `npm run build` or `ncc build src/index.ts`).
4. Update `README.md` to note the Node 24 requirement.
5. Open a PR titled "Migrate to Node 24 runtime".
6. Smoke-test on a runner via a workflow that pins to the PR's SHA.
7. Tag a new **major** release. Bumping the runtime is a breaking change for self-hosted consumers on dropped platforms — do not ship it as a minor bump.
8. Update the moving major tag (e.g. `v5`) to point at the new release once the changelog is reviewed.
9. Mention the change prominently in release notes — consumers will read these to decide whether to bump.

## What to do if dependencies block the bump

If a runtime dependency does not yet support Node 24:

- Replace the dependency. Most popular npm packages already ship Node 24 support; if one does not, an alternative usually does.
- If replacement is not feasible, ship an interim release that still declares `node20` and document the blocker in an issue. Consumers can use `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true` to smoke-test against Node 24 even on the old declaration, which gives early signal.
- Keep the action published as Node 20 until the dependency lands. The runner-side `ACTIONS_ALLOW_USE_UNSECURE_NODE_VERSION=true` flag is a consumer-side escape hatch that works only until Node 20 is removed entirely — it is not a substitute for migrating the action.
