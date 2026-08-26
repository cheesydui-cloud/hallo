#!/bin/sh
# 交叉编译 Linux amd64/arm64 发布包到 dist/release/<version>/
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"
VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
fi
OUT="$ROOT/dist/release/$VERSION"
rm -rf "$OUT"
mkdir -p "$OUT"

sum() {
  dir=$(dirname "$1")
  base=$(basename "$1")
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$dir" && sha256sum "$base" > "$base.sha256")
  else
    (cd "$dir" && shasum -a 256 "$base" > "$base.sha256")
  fi
}

build_web() {
  if [ ! -d web/node_modules ]; then
    (cd web && npm install)
  fi
  (cd web && npm run build)
}

pack() {
  goos=$1
  goarch=$2
  role=$3
  bin=$4
  pkg="hallo"
  [ "$role" = "agent" ] && pkg="hallo-agent"
  name="${pkg}-${goos}-${goarch}"
  echo "build $name"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o "$OUT/$bin" ./cmd/$bin
  cp README.md "$OUT/" 2>/dev/null || true
  extra="README.md"
  if [ "$role" = "panel" ]; then
    cp scripts/install.sh "$OUT/install.sh"
    extra="README.md install.sh"
  fi
  # shellcheck disable=SC2086
  tar -C "$OUT" -czf "$OUT/${name}.tar.gz" $bin $extra
  sum "$OUT/${name}.tar.gz"
  rm -f "$OUT/$bin"
}

build_web
for arch in amd64 arm64; do
  pack linux "$arch" panel hallo
  pack linux "$arch" agent hallo-agent
done
cp scripts/install.sh "$OUT/install.sh"
echo "产物：$OUT"
ls -l "$OUT"
