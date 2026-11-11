#!/usr/bin/env bash
# Builds the markdown-raw-editor plugin package.
#
# The editor is frontend-only, but the plugin system requires a WASM entry
# point, so we compile the stub in backend/ and package the static editor
# alongside it. Output: markdown-raw-editor.zip
set -euo pipefail

cd "$(dirname "$0")"

echo "Building markdown-raw-editor plugin..."

rm -rf dist
rm -f markdown-raw-editor.zip
mkdir -p dist/assets

echo "Building backend WASM stub..."
(cd backend && GOOS=wasip1 GOARCH=wasm go build -o ../dist/plugin.wasm main.go)

echo "Copying manifest and editor assets..."
cp manifest.json dist/
cp assets/editor.html assets/editor.css assets/editor.js dist/assets/

echo "Packaging..."
(cd dist && zip -q -r ../markdown-raw-editor.zip manifest.json plugin.wasm assets)

echo ""
echo "✓ Built markdown-raw-editor.zip"
unzip -l markdown-raw-editor.zip
echo ""
echo "Install locally:"
echo "  rm -rf plugins/markdown-raw-editor"
echo "  mkdir -p plugins/markdown-raw-editor"
echo "  unzip -o markdown-raw-editor.zip -d plugins/markdown-raw-editor/"
