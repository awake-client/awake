#!/bin/sh
# Prepares a release: turns the Unreleased section of CHANGELOG.md into the
# section of the new version, commits it, tags it and, once confirmed, pushes
# both. The tag then builds and publishes the release, with that changelog
# section as its notes.
#
#   build/release.sh 0.10.0            prepare, then ask before pushing
#   build/release.sh 0.10.0 --dry-run  print the new section, change nothing
#
# The changelog is written before the tag on purpose: the application reads
# the changelog of a release tag to show what an update brings.
set -eu

CHANGELOG=CHANGELOG.md
BRANCH=main
REMOTE=origin

die() {
  printf 'release: %s\n' "$1" >&2
  exit 1
}

VERSION="${1:-}"
DRY_RUN=false
[ "${2:-}" = "--dry-run" ] && DRY_RUN=true

VERSION="${VERSION#v}"
printf '%s' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' ||
  die "usage: build/release.sh <version> [--dry-run], such as 0.10.0"
TAG="v$VERSION"
TODAY=$(date -u +%Y-%m-%d)

cd "$(git rev-parse --show-toplevel)"

[ -f "$CHANGELOG" ] || die "$CHANGELOG is missing"
if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
  die "tag $TAG already exists"
fi
grep -q "^## \[$VERSION\]" "$CHANGELOG" && die "$CHANGELOG already has a $VERSION section"

# The changes of the release: the Unreleased section, up to the next version.
NOTES=$(awk '
  /^## \[Unreleased\]/ { inside = 1; next }
  /^## \[/ && inside { exit }
  /^\[[^]]+\]: / { next }
  inside { print }
' "$CHANGELOG" | sed -e '/./,$!d')
[ -n "$(printf '%s' "$NOTES" | tr -d '[:space:]')" ] ||
  die "the Unreleased section of $CHANGELOG is empty: describe the changes first"

if $DRY_RUN; then
  printf '## [%s] - %s\n\n%s\n' "$VERSION" "$TODAY" "$NOTES"
  exit 0
fi

[ "$(git rev-parse --abbrev-ref HEAD)" = "$BRANCH" ] || die "release from $BRANCH"
[ -z "$(git status --porcelain)" ] || die "commit or stash your changes first"
git fetch -q "$REMOTE" "$BRANCH"
[ "$(git rev-parse HEAD)" = "$(git rev-parse "$REMOTE/$BRANCH")" ] ||
  die "$BRANCH differs from $REMOTE/$BRANCH: pull or push first"

PREVIOUS=$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)
REPOSITORY=$(sed -n 's|^\[Unreleased\]: \(https://[^ ]*\)/compare/.*|\1|p' "$CHANGELOG")
[ -n "$REPOSITORY" ] || REPOSITORY="https://github.com/awake-client/awake"

# Opens an empty Unreleased section above the new version, and moves the
# comparison links: Unreleased now starts at the new tag.
awk -v version="$VERSION" -v today="$TODAY" -v tag="$TAG" \
  -v previous="$PREVIOUS" -v repository="$REPOSITORY" '
  /^## \[Unreleased\]/ {
    print
    print ""
    printf "## [%s] - %s\n", version, today
    next
  }
  /^\[Unreleased\]: / {
    printf "[Unreleased]: %s/compare/%s...HEAD\n", repository, tag
    if (previous != "") {
      printf "[%s]: %s/compare/%s...%s\n", version, repository, previous, tag
    } else {
      printf "[%s]: %s/releases/tag/%s\n", version, repository, tag
    }
    next
  }
  { print }
' "$CHANGELOG" >"$CHANGELOG.tmp"
mv "$CHANGELOG.tmp" "$CHANGELOG"

git add "$CHANGELOG"
git commit -q -m "chore(release): $VERSION"
git tag -a "$TAG" -m "Awake $VERSION"

printf '\nPrepared %s:\n\n' "$TAG"
git --no-pager show --stat --format='  %h %s' HEAD
printf '\nPush %s and %s to %s? This publishes the release. [y/N] ' "$BRANCH" "$TAG" "$REMOTE"
read -r answer
case "$answer" in
  y | Y | yes)
    git push --atomic "$REMOTE" "$BRANCH" "$TAG"
    printf 'Pushed. The release workflow builds and publishes %s.\n' "$TAG"
    ;;
  *)
    printf 'Not pushed. To publish later: git push --atomic %s %s %s\n' "$REMOTE" "$BRANCH" "$TAG"
    printf 'To undo: git tag -d %s && git reset --hard HEAD~1\n' "$TAG"
    ;;
esac
