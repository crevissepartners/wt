#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

gomod_file="./go.mod"
release_please_config="./release-please-config.json"
release_please_manifest="./.release-please-manifest.json"
changelog="./CHANGELOG.md"
buildinfo_file="./internal/buildinfo/buildinfo.go"
expected_module="github.com/crevissepartners/wt"

if [ ! -f "$gomod_file" ]; then
  echo "premerge: missing go.mod file" >&2
  exit 1
fi

if ! grep -Eq "^module[[:space:]]+$expected_module\$" "$gomod_file"; then
  echo "premerge: go.mod module path must be '$expected_module'" >&2
  exit 1
fi

if [ ! -f "$release_please_config" ]; then
  echo "premerge: missing release-please config: $release_please_config" >&2
  exit 1
fi

if [ ! -f "$release_please_manifest" ]; then
  echo "premerge: missing release-please manifest: $release_please_manifest" >&2
  exit 1
fi

if [ ! -f "$changelog" ]; then
  echo "premerge: missing CHANGELOG.md" >&2
  exit 1
fi

if [ ! -f "$buildinfo_file" ]; then
  echo "premerge: missing buildinfo version file: $buildinfo_file" >&2
  exit 1
fi

version="$(sed -n 's/.*Version = "\([^"]*\)".*/\1/p' "$buildinfo_file" | head -n 1)"
if ! printf "%s" "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([\-][0-9A-Za-z.-]+)?([+][0-9A-Za-z.-]+)?$'; then
  echo "premerge: buildinfo.Version is not valid semver: $version" >&2
  exit 1
fi

if ! grep -q 'x-release-please-version' "$buildinfo_file"; then
  echo "premerge: buildinfo.Version must keep x-release-please-version marker" >&2
  exit 1
fi

manifest_version="$(sed -n 's/.*"\.": "\([^"]*\)".*/\1/p' "$release_please_manifest" | head -n 1)"
if [ "$manifest_version" != "$version" ]; then
  echo "premerge: release-please manifest version must match buildinfo.Version (manifest=$manifest_version buildinfo=$version)" >&2
  exit 1
fi

exit 0
