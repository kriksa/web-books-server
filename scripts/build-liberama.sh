#!/usr/bin/env bash
# Build Liberama and copy into internal/spaembed/spa/liberama (after `make web`).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LIB="${LIBERAMA_ROOT:-$ROOT/third_party/liberama}"
OUT="$ROOT/internal/spaembed/spa/liberama"
ASSETS="$ROOT/assets"

if [[ ! -d "$LIB" ]]; then
  echo "Clone Liberama to third_party/liberama or set LIBERAMA_ROOT" >&2
  exit 1
fi

LOCAL_TMP="${TMPDIR:-$ROOT/.tmp}"
mkdir -p "$LOCAL_TMP"
export TMPDIR="$LOCAL_TMP" TEMP="$LOCAL_TMP" TMP="$LOCAL_TMP"

mkdir -p "$LIB/build"
echo "module.exports = 'liberama';" > "$LIB/build/appdir.js"

cd "$LIB"
npm install --ignore-scripts
if [[ -f node_modules/webpack/bin/webpack.js ]]; then
  node node_modules/webpack/bin/webpack.js --config build/webpack.prod.config.js
else
  npm run build:client
fi

mkdir -p "$OUT"
cp -R dist/tmp/public/liberama/. "$OUT/"
cp dist/tmp/public/index.html "$OUT/index.html"
[[ -f dist/tmp/public/sw-register.js ]] && cp dist/tmp/public/sw-register.js "$OUT/"
[[ -f dist/tmp/public/service-worker.js ]] && cp dist/tmp/public/service-worker.js "$OUT/"
if grep -q 'src="/sw-register.js"' "$OUT/index.html" 2>/dev/null; then
  sed -i 's@src="/sw-register.js"@src="/liberama/sw-register.js"@g' "$OUT/index.html"
fi
if [[ -f "$ASSETS/liberama-modern.css" ]]; then
  cp "$ASSETS/liberama-modern.css" "$OUT/web-books-modern.css"
fi
echo "Liberama copied to $OUT"
