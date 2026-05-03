#!/usr/bin/env python3
"""Apply Node 24 migration fixes from an audit.json produced by audit.py.

Usage: python apply_fix.py <audit.json> [--apply] [--repo-root .]

Default mode is dry-run: prints unified diffs of every fix it would make and
writes nothing. Pass --apply to write the changes in place.

Handles two finding kinds:
  - workflow-uses, state=node20-fixable
      Rewrites the `uses: <slug>@<ref>` line in the workflow file to the
      recommended ref. If the original ref was a 40-char SHA, the recommended
      tag is resolved to its SHA via `gh api`. Comments and indentation on the
      line are preserved.
  - local-action, state=local-node20
      Rewrites `using: node20` to `using: node24` inside the action.yml's
      `runs:` block. Comments and quoting are preserved.

Findings in any other state are skipped with a warning.

Exit codes:
  0  every non-skipped finding was applied successfully (or dry-run finished cleanly)
  1  a fix failed to apply, OR one or more findings were skipped (e.g. node20-stuck)
"""

from __future__ import annotations

import argparse
import difflib
import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

SHA_RE = re.compile(r'^[0-9a-f]{40}$')

USES_LINE_RE = re.compile(
    r'^(?P<prefix>\s*-?\s*uses\s*:\s*)'
    r'(?P<value>[^\s#\'"][^#]*?|\'[^\']*\'|"[^"]*")'
    r'(?P<trailer>\s*(?:#.*)?)$'
)


def have_gh() -> bool:
    return shutil.which("gh") is not None


_TAG_SHA_CACHE: dict[tuple[str, str], str | None] = {}


def resolve_tag_to_sha(slug: str, tag: str) -> str | None:
    key = (slug, tag)
    if key in _TAG_SHA_CACHE:
        return _TAG_SHA_CACHE[key]
    # Try as a tag ref (annotated tags resolve via /git/refs/tags/<tag>; lightweight
    # tags resolve directly). Fall back to /commits/<tag> which works for both.
    p = subprocess.run(
        ["gh", "api", f"repos/{slug}/commits/{tag}", "-q", ".sha"],
        capture_output=True,
        text=True,
    )
    if p.returncode == 0:
        sha = p.stdout.strip()
        if SHA_RE.match(sha):
            _TAG_SHA_CACHE[key] = sha
            return sha
    _TAG_SHA_CACHE[key] = None
    return None


def strip_quotes(value: str) -> tuple[str, str]:
    """Return (inner, quote_char). quote_char is '' if unquoted."""
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "'\"":
        return value[1:-1], value[0]
    return value, ""


def rewrite_uses_value(
    original_value: str,
    new_ref: str,
) -> str:
    """Rewrite the @ref portion of a uses: value, preserving any quoting."""
    inner, quote = strip_quotes(original_value)
    if "@" not in inner:
        return original_value
    path, _, _old_ref = inner.partition("@")
    new_inner = f"{path}@{new_ref}"
    return f"{quote}{new_inner}{quote}"


def apply_workflow_fix(
    file_text: str,
    line_no: int,
    expected_slug: str,
    new_ref: str,
) -> tuple[str, str]:
    """Return (new_text, replaced_line) or raise ValueError if the line doesn't
    match the expected pattern."""
    lines = file_text.splitlines(keepends=True)
    if line_no < 1 or line_no > len(lines):
        raise ValueError(f"line {line_no} out of range (file has {len(lines)} lines)")
    raw = lines[line_no - 1]
    # Preserve the original line ending.
    eol = ""
    body = raw
    if body.endswith("\r\n"):
        eol, body = "\r\n", body[:-2]
    elif body.endswith("\n"):
        eol, body = "\n", body[:-1]
    m = USES_LINE_RE.match(body)
    if not m:
        raise ValueError(f"line {line_no} does not look like a uses: line: {body!r}")
    inner, _ = strip_quotes(m.group("value"))
    path, _, _ = inner.partition("@")
    if not path.startswith(expected_slug):
        raise ValueError(
            f"line {line_no} slug {path!r} does not match expected {expected_slug!r}"
        )
    new_value = rewrite_uses_value(m.group("value"), new_ref)
    new_line = f"{m.group('prefix')}{new_value}{m.group('trailer')}{eol}"
    lines[line_no - 1] = new_line
    return "".join(lines), new_line.rstrip("\r\n")


USING_LINE_RE = re.compile(
    r'^(?P<prefix>\s+using\s*:\s*)'
    r'(?P<quote>[\'"]?)node20(?P=quote)'
    r'(?P<trailer>\s*(?:#.*)?)$'
)


def apply_local_action_fix(file_text: str) -> tuple[str, int]:
    """Rewrite `using: node20` -> `using: node24` inside the runs: block.
    Returns (new_text, lines_changed)."""
    lines = file_text.splitlines(keepends=True)
    runs_idx = None
    runs_indent = -1
    for i, line in enumerate(lines):
        if re.match(r'^(\s*)runs\s*:\s*(?:#.*)?$', line):
            runs_idx = i
            runs_indent = len(re.match(r'^(\s*)', line).group(1))
            break
    if runs_idx is None:
        raise ValueError("no `runs:` block found in action.yml")
    changed = 0
    for j in range(runs_idx + 1, len(lines)):
        body = lines[j].rstrip("\r\n")
        eol = lines[j][len(body):]
        if body.strip() == "":
            continue
        # End of the runs: block: same-or-shallower indent on a non-empty line.
        leading = len(re.match(r'^(\s*)', body).group(1))
        if leading <= runs_indent and body.strip() and not body.startswith(" " * (runs_indent + 1)):
            break
        m = USING_LINE_RE.match(body)
        if m:
            new_body = f"{m.group('prefix')}{m.group('quote')}node24{m.group('quote')}{m.group('trailer')}"
            lines[j] = new_body + eol
            changed += 1
    if changed == 0:
        raise ValueError("no `using: node20` found inside runs: block")
    return "".join(lines), changed


def diff_text(path: str, before: str, after: str) -> str:
    return "".join(
        difflib.unified_diff(
            before.splitlines(keepends=True),
            after.splitlines(keepends=True),
            fromfile=f"a/{path}",
            tofile=f"b/{path}",
        )
    )


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("audit_json", type=Path, help="audit.json from audit.py")
    ap.add_argument("--apply", action="store_true", help="Write changes (default: dry-run, prints diffs)")
    ap.add_argument("--repo-root", type=Path, default=Path("."), help="Repo root for resolving file paths")
    args = ap.parse_args()

    if not args.audit_json.exists():
        print(f"ERROR: {args.audit_json} not found", file=sys.stderr)
        return 1

    findings = json.loads(args.audit_json.read_text(encoding="utf-8"))
    repo_root = args.repo_root.resolve()

    applied = 0
    skipped = 0
    failed = 0

    # Group workflow-uses findings by file so we apply all line edits to one buffer
    # before writing — avoids stale line numbers if multiple uses: lines change.
    by_file: dict[Path, list[dict]] = {}
    local_actions: list[dict] = []
    other_findings: list[dict] = []
    for f in findings:
        if f.get("kind") == "workflow-uses" and f.get("state") == "node20-fixable":
            by_file.setdefault(repo_root / f["file"], []).append(f)
        elif f.get("kind") == "local-action" and f.get("state") == "local-node20":
            local_actions.append(f)
        else:
            other_findings.append(f)

    for f in other_findings:
        state = f.get("state", "")
        if state in {"node24", "local-node24", "docker", "reusable-workflow", "local-action"}:
            continue  # nothing to do, not counted as skipped
        skipped += 1
        print(
            f"SKIP  {f.get('file')}:{f.get('line') or '-'}  "
            f"{f.get('slug') or f.get('raw')}  state={state}  notes={f.get('notes', '')}",
            file=sys.stderr,
        )

    # Workflow fixes
    for path, items in by_file.items():
        if not path.exists():
            print(f"ERROR: {path} not found", file=sys.stderr)
            failed += 1
            continue
        before = path.read_text(encoding="utf-8")
        text = before
        # Apply highest line numbers first so earlier line edits don't shift later ones.
        items_sorted = sorted(items, key=lambda x: x["line"], reverse=True)
        per_file_applied = 0
        for f in items_sorted:
            new_ref = f.get("recommended_ref", "")
            if not new_ref:
                print(f"ERROR: {f['file']}:{f['line']} no recommended_ref", file=sys.stderr)
                failed += 1
                continue
            if f.get("pin_style") == "sha":
                if not have_gh():
                    print(
                        f"ERROR: {f['file']}:{f['line']} pin_style=sha but `gh` not installed; "
                        "cannot resolve recommended tag to SHA",
                        file=sys.stderr,
                    )
                    failed += 1
                    continue
                sha = resolve_tag_to_sha(f["slug"], new_ref)
                if not sha:
                    print(
                        f"ERROR: {f['file']}:{f['line']} could not resolve {f['slug']}@{new_ref} to SHA",
                        file=sys.stderr,
                    )
                    failed += 1
                    continue
                new_ref_for_line = sha
            else:
                new_ref_for_line = new_ref
            try:
                text, _ = apply_workflow_fix(text, f["line"], f["slug"], new_ref_for_line)
                per_file_applied += 1
            except ValueError as exc:
                print(f"ERROR: {f['file']}:{f['line']} {exc}", file=sys.stderr)
                failed += 1

        if per_file_applied == 0 or text == before:
            continue

        diff = diff_text(str(path.relative_to(repo_root)), before, text)
        print(diff, end="" if diff.endswith("\n") else "\n")
        if args.apply:
            path.write_text(text, encoding="utf-8")
        applied += per_file_applied

    # Local action.yml fixes
    for f in local_actions:
        path = repo_root / f["file"]
        if not path.exists():
            print(f"ERROR: {path} not found", file=sys.stderr)
            failed += 1
            continue
        before = path.read_text(encoding="utf-8")
        try:
            text, changed = apply_local_action_fix(before)
        except ValueError as exc:
            print(f"ERROR: {f['file']} {exc}", file=sys.stderr)
            failed += 1
            continue
        if text == before:
            continue
        diff = diff_text(str(path.relative_to(repo_root)), before, text)
        print(diff, end="" if diff.endswith("\n") else "\n")
        if args.apply:
            path.write_text(text, encoding="utf-8")
        applied += changed

    mode = "applied" if args.apply else "would apply"
    print(
        f"\n{mode}: {applied}  skipped: {skipped}  failed: {failed}",
        file=sys.stderr,
    )
    if failed:
        return 1
    if skipped:
        # Skipped findings are surfaced for human attention — exit non-zero so CI can gate.
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
