#!/usr/bin/env python3
"""Audit GitHub Actions workflows and action.yml files for the Node 20 deprecation.

Usage: python audit.py <repo-root> [--out audit.json] [--no-network]

Walks <repo-root> for:
  .github/workflows/*.yml, *.yaml         -> every `uses:` line
  action.yml, action.yaml, **/action.yml  -> the `runs.using` field

For each external `uses: <slug>@<ref>`, calls `gh api` to fetch the action.yml at
that ref, reads runs.using, and (if node20) discovers the latest release on
Node 24 if one exists.

Emits a markdown table to stdout and structured findings to audit.json.

Requires: gh CLI, authenticated. Pure stdlib otherwise.

Exit codes:
  0  audit complete (regardless of findings)
  1  fatal (gh missing, repo path not a directory, etc.)
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import re
import shutil
import subprocess
import sys
from dataclasses import asdict, dataclass, field
from pathlib import Path

USES_RE = re.compile(
    r'^(?P<prefix>\s*-?\s*)uses\s*:\s*'
    r'(?P<value>[^\s#\'"][^#]*?|\'[^\']*\'|"[^"]*")'
    r'(?:\s+#.*)?\s*$'
)
USING_RE = re.compile(
    r'^\s+using\s*:\s*[\'"]?(?P<val>[A-Za-z0-9_.-]+)[\'"]?\s*(?:#.*)?$',
    re.M,
)
SHA_RE = re.compile(r'^[0-9a-f]{40}$')

NODE20 = "node20"
NODE24 = "node24"


@dataclass
class Finding:
    file: str
    line: int
    kind: str  # "workflow-uses" | "local-action"
    raw: str
    slug: str = ""
    sub_path: str = ""
    ref: str = ""
    pin_style: str = ""  # tag | sha | branch | unknown
    state: str = ""  # node24 | node20-fixable | node20-stuck | local-node20 |
                     # local-node24 | composite | docker | reusable-workflow |
                     # local-action | unresolved | skipped
    current_node: str = ""
    recommended_ref: str = ""
    recommended_node: str = ""
    notes: str = ""


# ---------------------------------------------------------------------------
# gh api helpers
# ---------------------------------------------------------------------------

def have_gh() -> bool:
    return shutil.which("gh") is not None


_FILE_CACHE: dict[tuple[str, str, str], str | None] = {}


def gh_api(path: str) -> tuple[int, str, str]:
    p = subprocess.run(
        ["gh", "api", path],
        capture_output=True,
        text=True,
    )
    return p.returncode, p.stdout, p.stderr


def fetch_action_yml(owner: str, repo: str, sub_path: str, ref: str) -> str | None:
    """Fetch action.yml or action.yaml from a repo at a specific ref. Cached."""
    key = (f"{owner}/{repo}", sub_path, ref)
    if key in _FILE_CACHE:
        return _FILE_CACHE[key]
    sub_path = sub_path.strip("/")
    for fname in ("action.yml", "action.yaml"):
        path_part = f"{sub_path}/{fname}" if sub_path else fname
        api_path = f"repos/{owner}/{repo}/contents/{path_part}"
        if ref:
            api_path += f"?ref={ref}"
        rc, out, _ = gh_api(api_path)
        if rc != 0:
            continue
        try:
            data = json.loads(out)
        except json.JSONDecodeError:
            continue
        if isinstance(data, dict) and data.get("encoding") == "base64":
            try:
                content = base64.b64decode(data["content"]).decode(
                    "utf-8", errors="replace"
                )
                _FILE_CACHE[key] = content
                return content
            except (KeyError, ValueError):
                continue
    _FILE_CACHE[key] = None
    return None


_LATEST_CACHE: dict[str, str | None] = {}


def fetch_latest_release_tag(owner: str, repo: str) -> str | None:
    key = f"{owner}/{repo}"
    if key in _LATEST_CACHE:
        return _LATEST_CACHE[key]
    rc, out, _ = gh_api(f"repos/{owner}/{repo}/releases/latest")
    if rc != 0:
        _LATEST_CACHE[key] = None
        return None
    try:
        data = json.loads(out)
    except json.JSONDecodeError:
        _LATEST_CACHE[key] = None
        return None
    tag = data.get("tag_name") if isinstance(data, dict) else None
    _LATEST_CACHE[key] = tag
    return tag


# ---------------------------------------------------------------------------
# Parsing
# ---------------------------------------------------------------------------

def extract_using(text: str) -> str | None:
    """Return the value of `using:` inside the first `runs:` block, or None."""
    # Find a `runs:` line at column 0 (or any column) followed by indented block
    runs_match = re.search(r'^(\s*)runs\s*:\s*(?:#.*)?$', text, re.M)
    if not runs_match:
        return None
    runs_indent = len(runs_match.group(1))
    start = runs_match.end()
    # Slice until a sibling top-level key (same or shallower indent than runs_indent)
    end = len(text)
    for m in re.finditer(r'^(\s*)\S', text[start:], re.M):
        if len(m.group(1)) <= runs_indent and start + m.start() != start:
            end = start + m.start()
            break
    block = text[start:end]
    using = USING_RE.search(block)
    return using.group("val").lower() if using else None


def strip_quotes(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "'\"":
        return value[1:-1]
    return value


def classify_pin(ref: str) -> str:
    if SHA_RE.match(ref):
        return "sha"
    if ref.startswith("v") and re.match(r'^v\d', ref):
        return "tag"
    if not ref:
        return "unknown"
    # Fallback: anything else (branch name, semver-without-v, exotic tag)
    return "tag" if re.match(r'^[\w.\-]+$', ref) else "unknown"


def split_uses(value: str) -> tuple[str, str, str, str]:
    """Return (kind, slug, sub_path, ref).

    kind is one of: 'external', 'local', 'docker', 'reusable-workflow', 'invalid'.
    """
    raw = strip_quotes(value)
    if raw.startswith("docker://"):
        return "docker", "", "", raw[len("docker://"):]
    if raw.startswith("./") or raw.startswith("../"):
        return "local", "", raw, ""
    if "@" not in raw:
        # Reusable workflow `uses: org/repo/.github/workflows/x.yml@ref` always has @.
        # Anything else without @ is unusual / invalid.
        return "invalid", "", "", raw
    path, _, ref = raw.partition("@")
    parts = path.split("/")
    if len(parts) < 2:
        return "invalid", "", "", raw
    owner, repo = parts[0], parts[1]
    sub = "/".join(parts[2:])
    slug = f"{owner}/{repo}"
    if sub.endswith(".yml") or sub.endswith(".yaml"):
        # Looks like a reusable workflow file ref.
        if "/.github/workflows/" in f"/{sub}":
            return "reusable-workflow", slug, sub, ref
    return "external", slug, sub, ref


# ---------------------------------------------------------------------------
# File walking
# ---------------------------------------------------------------------------

def find_workflow_files(root: Path) -> list[Path]:
    out: list[Path] = []
    for wd in root.rglob(".github/workflows"):
        if not wd.is_dir():
            continue
        for p in sorted(wd.iterdir()):
            if p.is_file() and p.suffix.lower() in {".yml", ".yaml"}:
                out.append(p)
    return out


def find_action_files(root: Path) -> list[Path]:
    out: list[Path] = []
    for name in ("action.yml", "action.yaml"):
        for p in root.rglob(name):
            # Skip anything inside node_modules, .git, vendor, etc.
            parts = set(p.parts)
            if {".git", "node_modules", "vendor", "dist"} & parts:
                continue
            out.append(p)
    return sorted(set(out))


def iter_uses_lines(path: Path) -> list[tuple[int, str]]:
    """Yield (line_number, raw_value) for every `uses:` line in the file."""
    out: list[tuple[int, str]] = []
    try:
        text = path.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        return out
    for i, line in enumerate(text.splitlines(), start=1):
        m = USES_RE.match(line)
        if m:
            out.append((i, m.group("value")))
    return out


# ---------------------------------------------------------------------------
# Classification
# ---------------------------------------------------------------------------

def classify_external(
    finding: Finding,
    network: bool,
) -> None:
    if not network:
        finding.state = "skipped"
        finding.notes = "network disabled"
        return
    owner, _, repo = finding.slug.partition("/")
    text = fetch_action_yml(owner, repo, finding.sub_path, finding.ref)
    if text is None:
        finding.state = "unresolved"
        finding.notes = "action.yml not reachable at this ref"
        return
    using = extract_using(text)
    if using is None:
        finding.state = "unresolved"
        finding.notes = "runs.using not found in action.yml"
        return
    finding.current_node = using
    if using == "docker":
        finding.state = "docker"
        return
    if using == "composite":
        finding.state = "composite"
        finding.notes = "composite action — its inner uses: refs decide effective Node"
        # Best-effort recommendation: latest tag.
        latest = fetch_latest_release_tag(owner, repo)
        if latest and latest != finding.ref:
            finding.recommended_ref = latest
        return
    if using == NODE24:
        finding.state = "node24"
        return
    if using == NODE20:
        latest = fetch_latest_release_tag(owner, repo)
        if not latest:
            finding.state = "node20-stuck"
            finding.notes = "no published release found"
            return
        latest_text = fetch_action_yml(owner, repo, finding.sub_path, latest)
        latest_using = extract_using(latest_text) if latest_text else None
        if latest_using == NODE24:
            finding.state = "node20-fixable"
            finding.recommended_ref = latest
            finding.recommended_node = NODE24
        else:
            finding.state = "node20-stuck"
            finding.recommended_ref = latest or ""
            finding.recommended_node = latest_using or ""
            finding.notes = "latest release still not on Node 24"
        return
    # Older or unknown runtime (node12/node16/etc.)
    finding.state = "node20-stuck" if using.startswith("node") else "unresolved"
    finding.notes = f"unexpected runs.using: {using}"


def classify_local_action(path: Path, repo_root: Path) -> Finding:
    rel = str(path.relative_to(repo_root))
    finding = Finding(
        file=rel,
        line=0,
        kind="local-action",
        raw=rel,
    )
    try:
        text = path.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        finding.state = "unresolved"
        finding.notes = "could not decode as UTF-8"
        return finding
    using = extract_using(text)
    if using is None:
        finding.state = "unresolved"
        finding.notes = "runs.using not found"
        return finding
    finding.current_node = using
    if using == NODE20:
        finding.state = "local-node20"
        finding.recommended_node = NODE24
    elif using == NODE24:
        finding.state = "local-node24"
    elif using == "composite":
        finding.state = "composite"
    elif using == "docker":
        finding.state = "docker"
    else:
        finding.state = "unresolved"
        finding.notes = f"unexpected runs.using: {using}"
    return finding


# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

STATE_PRIORITY = {
    "node20-fixable": 0,
    "local-node20": 1,
    "node20-stuck": 2,
    "unresolved": 3,
    "composite": 4,
    "reusable-workflow": 5,
    "local-action": 6,
    "docker": 7,
    "node24": 8,
    "local-node24": 9,
    "skipped": 10,
}


def render_markdown_table(findings: list[Finding]) -> str:
    if not findings:
        return "_No findings — repo has no GitHub Actions workflows or local actions._\n"
    cols = (
        "File",
        "Line",
        "Action",
        "Ref",
        "Pin",
        "State",
        "Current",
        "Recommended ref",
        "Notes",
    )
    rows = [cols, ("---",) * len(cols)]
    for f in findings:
        action = f.slug + (f"/{f.sub_path}" if f.sub_path else "") if f.slug else f.raw
        rows.append((
            f.file,
            str(f.line) if f.line else "-",
            action,
            f.ref or "-",
            f.pin_style or "-",
            f.state or "-",
            f.current_node or "-",
            f.recommended_ref or "-",
            f.notes or "",
        ))
    widths = [max(len(str(r[i])) for r in rows) for i in range(len(cols))]
    out_lines = []
    for r in rows:
        out_lines.append(
            "| " + " | ".join(str(r[i]).ljust(widths[i]) for i in range(len(cols))) + " |"
        )
    return "\n".join(out_lines) + "\n"


def render_summary(findings: list[Finding]) -> str:
    counts: dict[str, int] = {}
    for f in findings:
        counts[f.state] = counts.get(f.state, 0) + 1
    if not counts:
        return ""
    parts = [f"{counts[k]} {k}" for k in sorted(counts, key=lambda s: STATE_PRIORITY.get(s, 99))]
    return "Summary: " + ", ".join(parts) + "\n"


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("repo_root", type=Path, help="Path to the repo to audit")
    ap.add_argument("--out", type=Path, default=Path("audit.json"), help="Path to write audit JSON")
    ap.add_argument(
        "--no-network",
        action="store_true",
        help="Skip gh api calls (smoke-test mode; states will be 'skipped' for external uses)",
    )
    args = ap.parse_args()

    if not args.repo_root.is_dir():
        print(f"ERROR: {args.repo_root} is not a directory", file=sys.stderr)
        return 1

    network = not args.no_network
    if network and not have_gh():
        print(
            "ERROR: `gh` CLI not found on PATH. Install it (https://cli.github.com) or rerun with --no-network.",
            file=sys.stderr,
        )
        return 1

    findings: list[Finding] = []

    # 1. Workflow files
    for wf in find_workflow_files(args.repo_root):
        rel = str(wf.relative_to(args.repo_root))
        for line, value in iter_uses_lines(wf):
            kind, slug, sub, ref = split_uses(value)
            f = Finding(
                file=rel,
                line=line,
                kind="workflow-uses",
                raw=strip_quotes(value),
                slug=slug,
                sub_path=sub,
                ref=ref,
                pin_style=classify_pin(ref) if ref else "unknown",
            )
            if kind == "docker":
                f.state = "docker"
            elif kind == "local":
                f.state = "local-action"
                f.notes = f"local action ref: {sub}"
            elif kind == "reusable-workflow":
                f.state = "reusable-workflow"
                f.notes = "reusable workflow file (not affected by Node deprecation)"
            elif kind == "invalid":
                f.state = "unresolved"
                f.notes = "could not parse as <slug>@<ref>"
            else:
                classify_external(f, network)
            findings.append(f)

    # 2. Local action.yml files
    for ap_path in find_action_files(args.repo_root):
        findings.append(classify_local_action(ap_path, args.repo_root))

    findings.sort(key=lambda f: (STATE_PRIORITY.get(f.state, 99), f.file, f.line))

    # Output
    print(render_markdown_table(findings))
    summary = render_summary(findings)
    if summary:
        print(summary)

    args.out.write_text(
        json.dumps([asdict(f) for f in findings], indent=2) + "\n",
        encoding="utf-8",
    )
    print(f"Wrote {len(findings)} finding(s) to {args.out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
