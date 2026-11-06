#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT=${OUT:-"$ROOT/dist"}
GOOS=${GOOS:-$(go env GOOS)}
GOARCH=${GOARCH:-$(go env GOARCH)}

if [ "$GOOS" = "windows" ]; then
    echo "use scripts/package.ps1 for Windows packages" >&2
    exit 1
fi

VERSION=${VERSION:-0.1.0}
case "$VERSION" in *[!A-Za-z0-9._-]*) echo 'invalid version' >&2; exit 1;; esac
NAME="game-l10n-updater_${VERSION}_${GOOS}_${GOARCH}"

sha256_hex() {
    if command -v sha256sum >/dev/null 2>&1; then
        hash_line=$(sha256sum "$1") || return 1
    elif command -v shasum >/dev/null 2>&1; then
        hash_line=$(shasum -a 256 "$1") || return 1
    else
        echo 'sha256sum or shasum is required' >&2
        return 1
    fi
    printf '%s\n' "${hash_line%% *}"
}

mkdir -p "$OUT"
cd "$ROOT"
OUT=$(CDPATH= cd -- "$OUT" && pwd)
STAGE=$(mktemp -d "$OUT/stage-XXXXXXXX")
trap 'rm -rf "$STAGE"' EXIT
mkdir -p "$STAGE/$NAME"

CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -trimpath -buildvcs=false -ldflags "-s -w" -o "$STAGE/$NAME/gamel10n" ./cmd/gamel10n

cp README.md LICENSE THIRD_PARTY_NOTICES.md "$STAGE/$NAME/"
for tree in docs examples godot LICENSES; do
  if [ -d "$tree" ]; then
    tar --exclude='.godot' --exclude='*.import' --exclude='*.translation' --exclude='*.uid' -cf - "$tree" | tar -xf - -C "$STAGE/$NAME"
  fi
done
tar -czf "$OUT/$NAME.tar.gz" -C "$STAGE" "$NAME"

sum=$(sha256_hex "$OUT/$NAME.tar.gz")
printf '%s  %s\n' "$sum" "$NAME.tar.gz" > "$OUT/SHA256SUMS.txt"
echo "built $OUT/$NAME.tar.gz"
echo "wrote $OUT/SHA256SUMS.txt"
