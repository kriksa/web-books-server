#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$SCRIPT_DIR"
while [ "$ROOT_DIR" != "/" ] && [ ! -f "$ROOT_DIR/go.mod" ]; do
  ROOT_DIR="$(dirname "$ROOT_DIR")"
done
[ -f "$ROOT_DIR/go.mod" ] || {
  echo "❌ go.mod not found (cannot locate project root)."
  exit 1
}

NEW_VERSION="${1:-}"

# Статика для go:embed (пакет internal/spaembed, каталог spa/).
EMBED_SPA_DIR="$ROOT_DIR/internal/spaembed/spa"
WEB_DIR="$ROOT_DIR/web"
ASSETS_DIR="$ROOT_DIR/assets"
# Liberama: third_party/liberama или legacy liberama-1.3.2 в корне.
if [ -z "${LIBERAMA_ROOT:-}" ]; then
  if [ -d "$ROOT_DIR/third_party/liberama" ]; then
    LIBERAMA_ROOT="$ROOT_DIR/third_party/liberama"
  else
    LIBERAMA_ROOT="$ROOT_DIR/liberama-1.3.2"
  fi
fi

echo "==> Build started"

command -v node >/dev/null 2>&1 || { echo "❌ node not found"; exit 1; }
command -v npm >/dev/null 2>&1 || { echo "❌ npm not found"; exit 1; }
command -v go >/dev/null 2>&1 || { echo "❌ go not found"; exit 1; }
echo "Node: $(node --version)"
echo "npm:  $(npm --version)"
echo "Go:   $(go version)"

echo "==> Preparing public assets for Vite (web/public)"
rm -rf "$WEB_DIR/public"
mkdir -p "$WEB_DIR/public/backgrounds"
if [ -f "$ASSETS_DIR/favicon.svg" ]; then
  cp "$ASSETS_DIR/favicon.svg" "$WEB_DIR/public/favicon.svg"
else
  echo "⚠️ Missing $ASSETS_DIR/favicon.svg"
fi
if [ -f "$ASSETS_DIR/favicon.ico" ]; then
  cp "$ASSETS_DIR/favicon.ico" "$WEB_DIR/public/favicon.ico"
fi
if [ -f "$ASSETS_DIR/apple-touch-icon.png" ]; then
  cp "$ASSETS_DIR/apple-touch-icon.png" "$WEB_DIR/public/apple-touch-icon.png"
fi
if [ -d "$ASSETS_DIR/backgrounds" ]; then
  cp -R "$ASSETS_DIR/backgrounds/." "$WEB_DIR/public/backgrounds/"
fi

echo "==> Frontend version"
if [ -n "$NEW_VERSION" ]; then
  if [[ "$NEW_VERSION" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
    # remove leading v for package.json, keep in Vite (it adds v prefix itself)
    CLEAN_VERSION="${NEW_VERSION#v}"
    if [ -f "$WEB_DIR/package.json" ]; then
      tmp_pkg="$WEB_DIR/package.tmp.json"
      if command -v jq >/dev/null 2>&1; then
        jq --arg v "$CLEAN_VERSION" '.version = $v' "$WEB_DIR/package.json" > "$tmp_pkg"
        mv "$tmp_pkg" "$WEB_DIR/package.json"
        echo "  Using overridden frontend version: $CLEAN_VERSION (from argument '$NEW_VERSION')"
      else
        # Fallback: naive in-place replace of existing version field
        if grep -q '"version"' "$WEB_DIR/package.json"; then
          sed -i.bak -E "s/\"version\"\\s*:\\s*\"[^\"]*\"/\"version\": \"${CLEAN_VERSION//\//\\/}\"/" "$WEB_DIR/package.json" || true
          echo "  Using overridden frontend version (sed): $CLEAN_VERSION (from argument '$NEW_VERSION')"
        else
          echo "⚠️ jq is missing and package.json has no version field; cannot safely set version"
        fi
      fi
    else
      echo "⚠️ $WEB_DIR/package.json not found; cannot set frontend version"
    fi
  else
    echo "⚠️ Invalid version format '$NEW_VERSION' (expected SemVer like 1.2.3 or v1.2.3); keeping existing version"
  fi
else
  if [ -f "$WEB_DIR/package.json" ]; then
    CURRENT_VERSION="$(grep -E '\"version\"\\s*:' \"$WEB_DIR/package.json\" | head -n1 | sed -E 's/.*\"version\"\\s*:\\s*\"([^\"]*)\".*/\\1/' || true)"
    if [ -n "$CURRENT_VERSION" ]; then
      echo "  Using existing frontend version: $CURRENT_VERSION"
    else
      echo "  Frontend version: (not set in package.json)"
    fi
  fi
fi

echo "==> Building frontend (Vite: web/ → internal/spaembed/spa)"
[ -f "$WEB_DIR/package.json" ] || { echo "❌ Missing $WEB_DIR/package.json"; exit 1; }
mkdir -p "$EMBED_SPA_DIR"
cd "$WEB_DIR"
npm ci

# esbuild optionalDependencies: heal missing platform package (Vite MODULE_NOT_FOUND).
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="x64" ;;
  aarch64|arm64) arch="arm64" ;;
esac
esbuild_pkg="@esbuild/${os}-${arch}"
esbuild_bin_1="$WEB_DIR/node_modules/${esbuild_pkg}/bin/esbuild"
esbuild_bin_2="$WEB_DIR/node_modules/${esbuild_pkg}/esbuild/bin/esbuild"

npm rebuild esbuild >/dev/null 2>&1 || true
if [ ! -f "$esbuild_bin_1" ] && [ ! -f "$esbuild_bin_2" ]; then
  echo "⚠️ esbuild binary missing; installing ${esbuild_pkg}"
  npm install --no-save "${esbuild_pkg}"
  npm rebuild esbuild >/dev/null 2>&1 || true
fi

if [ -f "$WEB_DIR/node_modules/vite/dist/node/cli.js" ]; then
  node "$WEB_DIR/node_modules/vite/dist/node/cli.js" build
elif [ -f "$WEB_DIR/node_modules/vite/dist/node/cli.mjs" ]; then
  node "$WEB_DIR/node_modules/vite/dist/node/cli.mjs" build
else
  npm run build
fi

[ -f "$EMBED_SPA_DIR/index.html" ] || { echo "❌ Missing $EMBED_SPA_DIR/index.html"; exit 1; }
[ -f "$EMBED_SPA_DIR/reader.html" ] || { echo "❌ Missing $EMBED_SPA_DIR/reader.html"; exit 1; }
echo "✅ Frontend assets ready:"
ls -1 "$EMBED_SPA_DIR" | head -50 | sed 's/^/  - /'

echo "==> Building Liberama client (for /liberama)"
[ -d "$LIBERAMA_ROOT" ] || { echo "❌ Missing Liberama dir: $LIBERAMA_ROOT (задайте LIBERAMA_ROOT)"; exit 1; }
[ -f "$LIBERAMA_ROOT/package.json" ] || { echo "❌ Missing $LIBERAMA_ROOT/package.json"; exit 1; }

LOCAL_TMP_DIR="$ROOT_DIR/.tmp"
mkdir -p "$LOCAL_TMP_DIR"
export TMPDIR="$LOCAL_TMP_DIR"
export TEMP="$LOCAL_TMP_DIR"
export TMP="$LOCAL_TMP_DIR"

mkdir -p "$LIBERAMA_ROOT/build"
cat > "$LIBERAMA_ROOT/build/appdir.js" << 'EOF'
module.exports = 'liberama';
EOF

cd "$LIBERAMA_ROOT"
npm install --ignore-scripts
if [ -f "$LIBERAMA_ROOT/node_modules/webpack/bin/webpack.js" ]; then
  node "$LIBERAMA_ROOT/node_modules/webpack/bin/webpack.js" --config build/webpack.prod.config.js
else
  npm run build:client
fi

rm -rf "$EMBED_SPA_DIR/liberama"
mkdir -p "$EMBED_SPA_DIR/liberama"

cp -R "$LIBERAMA_ROOT/dist/tmp/public/liberama/." "$EMBED_SPA_DIR/liberama/"
cp "$LIBERAMA_ROOT/dist/tmp/public/index.html" "$EMBED_SPA_DIR/liberama/index.html"

if [ -f "$LIBERAMA_ROOT/dist/tmp/public/sw-register.js" ]; then
  cp "$LIBERAMA_ROOT/dist/tmp/public/sw-register.js" "$EMBED_SPA_DIR/liberama/sw-register.js"
fi
if [ -f "$LIBERAMA_ROOT/dist/tmp/public/service-worker.js" ]; then
  cp "$LIBERAMA_ROOT/dist/tmp/public/service-worker.js" "$EMBED_SPA_DIR/liberama/service-worker.js"
fi

if grep -q 'src="/sw-register.js"' "$EMBED_SPA_DIR/liberama/index.html"; then
  sed -i 's@src="/sw-register.js"@src="/liberama/sw-register.js"@g' "$EMBED_SPA_DIR/liberama/index.html"
fi

if [ -f "$ASSETS_DIR/liberama-modern.css" ]; then
  cp "$ASSETS_DIR/liberama-modern.css" "$EMBED_SPA_DIR/liberama/web-books-modern.css"
  if ! grep -q "web-books-modern.css" "$EMBED_SPA_DIR/liberama/index.html"; then
    perl -0777 -i -pe 's@</head>@  <link rel="stylesheet" href="/liberama/web-books-modern.css" />\n</head>@s' "$EMBED_SPA_DIR/liberama/index.html"
  fi
fi

[ -f "$EMBED_SPA_DIR/liberama/index.html" ] || { echo "❌ Missing $EMBED_SPA_DIR/liberama/index.html"; exit 1; }
echo "✅ Liberama assets ready:"
ls -1 "$EMBED_SPA_DIR/liberama" | head -50 | sed 's/^/  - /'

echo "==> Building Go binary (cmd/web_books → bin/web_books)"
cd "$ROOT_DIR"
# Phase-3 migration: config/DB live in internal/config and internal/storage (wrappers in internal/app/aliases.go).
for legacy in internal/app/config.go internal/app/db.go; do
  if [ -f "$legacy" ]; then
    echo "⚠️  Removing obsolete $legacy"
    rm -f "$legacy"
  fi
done
mkdir -p "$ROOT_DIR/bin"
go mod tidy
go build -ldflags="-s -w" -o bin/web_books ./cmd/web_books

echo ""
echo "✅ Build complete"
echo "Binary: $ROOT_DIR/bin/web_books"
echo "Embedded static: $EMBED_SPA_DIR"
echo ""
