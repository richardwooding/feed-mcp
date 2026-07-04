#!/usr/bin/env sh
# sync-gloam.sh — vendor gloam.css and gloam.js from the canonical repo.
#
# gloam lives at github.com/richardwooding/gloam; consumers keep a *copy* of
# gloam.css/gloam.js next to their page. Run this to refresh that copy so the
# consumer doesn't drift from the design system.
#
# Usage:
#   sync-gloam.sh [target-dir] [ref]
#     target-dir  where gloam.css/gloam.js live (default: the script's own dir)
#     ref         branch or commit to sync from (default: main)
#
# Both files are fetched from the same resolved commit, and the commit is
# recorded in <target-dir>/.gloam-version for drift detection.
set -eu

REPO="richardwooding/gloam"
DEFAULT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
TARGET="${1:-$DEFAULT_DIR}"
REF="${2:-main}"
BASE="https://raw.githubusercontent.com/${REPO}"

[ -d "$TARGET" ] || { echo "sync-gloam: target dir not found: $TARGET" >&2; exit 1; }

# Resolve the ref to a commit SHA so both files come from one commit (no race
# if the branch moves mid-sync). Query the exact ref paths so we don't match
# other refs merely ending in $REF, and take the last line so an annotated tag
# resolves to its dereferenced commit (refs/tags/x^{}) rather than the tag
# object. A raw SHA (which matches no ref path) falls back to itself.
SHA="$(git ls-remote "https://github.com/${REPO}.git" "refs/heads/${REF}" "refs/tags/${REF}" 2>/dev/null | tail -1 | cut -f1)"
[ -n "$SHA" ] || SHA="$REF"

# Download to temp files and swap them in only once both succeed, so an
# interrupted or failed sync never leaves a half-updated pair.
for f in gloam.css gloam.js; do
  echo "↓ ${f} @ ${SHA}"
  curl -fsSL "${BASE}/${SHA}/${f}" -o "${TARGET}/${f}.tmp"
done
for f in gloam.css gloam.js; do
  mv "${TARGET}/${f}.tmp" "${TARGET}/${f}"
done

printf 'repo=%s\nref=%s\ncommit=%s\n' "$REPO" "$REF" "$SHA" > "${TARGET}/.gloam-version"
echo "✓ gloam synced to ${TARGET} (${SHA})"
