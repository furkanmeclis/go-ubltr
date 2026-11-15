#!/bin/sh
# Tags every library module at HEAD and pushes the tags.
# Usage: scripts/tag-release.sh v0.1.0
#
# Nested modules get "<dir>/<version>" tags; the root module gets
# "<version>". The root tag is pushed last so the Release workflow sees
# every module tag when it writes the release notes.
set -eu
version="${1:?usage: scripts/tag-release.sh vX.Y.Z}"
case "$version" in
	v[0-9]*.[0-9]*.[0-9]*) ;;
	*) echo "version must look like v1.2.3" >&2; exit 1 ;;
esac
cd "$(dirname "$0")/.."
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
	echo "working tree has uncommitted changes" >&2
	exit 1
fi
git fetch --quiet origin main
if [ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]; then
	echo "HEAD is not origin/main; pull or push first" >&2
	exit 1
fi
nested=""
for dir in $(scripts/modules.sh --libs); do
	[ "$dir" = "." ] && continue
	tag="$dir/$version"
	git tag -a "$tag" -m "$tag"
	nested="$nested refs/tags/$tag"
done
git tag -a "$version" -m "go-ubltr $version"
# shellcheck disable=SC2086
git push origin $nested
git push origin "refs/tags/$version"
echo "pushed $version and$(echo "$nested" | wc -w) module tags"
