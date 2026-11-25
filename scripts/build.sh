#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
export GOCACHE="${GOCACHE:-$repo_root/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$repo_root/.cache/go-mod}"
export GOTOOLCHAIN=auto

npm --prefix web run build

if [[ $# -gt 1 || ($# -eq 1 && "$1" != "--all") ]]; then
  echo 'Usage: bash scripts/build.sh [--all]' >&2
  exit 2
fi

if [[ "${1:-}" == "--all" ]]; then
  targets=(darwin/arm64 darwin/amd64 linux/amd64 linux/arm64)
else
  targets=("$(go env GOOS)/$(go env GOARCH)")
fi

for target in "${targets[@]}"; do
  target_os="${target%/*}"
  target_arch="${target#*/}"
  case "$target" in
    darwin/arm64|darwin/amd64|linux/amd64|linux/arm64) ;;
    *) echo "Unsupported ReaderHub build target: $target" >&2; exit 2 ;;
  esac
  output_dir="$repo_root/dist/$target_os-$target_arch"
  mkdir -p "$output_dir"
  CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -o "$output_dir/readerhub-cli" ./cmd/readerhub-cli
  if [[ "$target_os" == darwin ]]; then
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -o "$output_dir/readerhub" ./cmd/readerhub
  fi
  echo "Built $target in $output_dir"
done
