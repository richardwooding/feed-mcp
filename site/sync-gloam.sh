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
# if the branch moves mid-sync). A raw SHA passed as $REF resolves to itself.
SHA="$(git ls-remote "https://github.com/${REPO}.git" "$REF" 2>/dev/null | head -1 | cut -f1)"
[ -n "$SHA" ] || SHA="$REF"

for f in gloam.css gloam.js; do
  echo "↓ ${f} @ ${SHA}"
  curl -fsSL "${BASE}/${SHA}/${f}" -o "${TARGET}/${f}"
done

printf 'repo=%s\nref=%s\ncommit=%s\n' "$REPO" "$REF" "$SHA" > "${TARGET}/.gloam-version"
echo "✓ gloam synced to ${TARGET} (${SHA})"
