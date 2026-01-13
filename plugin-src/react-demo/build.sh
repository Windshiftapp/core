#!/bin/bash
set -e

echo "Building react-demo plugin..."

# Clean previous builds
echo "Cleaning previous build artifacts..."
rm -rf dist/
rm -f react-demo.zip

# Create dist directory structure
mkdir -p dist/assets

# Build frontend
echo "Building frontend..."
cd frontend

# Clean install dependencies
echo "Installing dependencies..."
rm -rf node_modules package-lock.json
npm install

echo "Running Vite build..."
npm run build
cd ..

# Vite outputs to dist/ with assets in dist/assets/
# We need to copy the HTML to assets and fix the path
echo "Preparing frontend assets..."
cp dist/index.html dist/assets/index.html

# Fix the script and link paths to be relative (remove /assets/ prefix)
echo "Fixing asset paths..."
sed -i '' 's|src="/assets/|src="./|g' dist/assets/index.html
sed -i '' 's|href="/assets/|href="./|g' dist/assets/index.html

# Clean up root index.html that we don't need
rm -f dist/index.html

# Build backend WASM
echo "Building backend WASM..."
cd backend
GOOS=wasip1 GOARCH=wasm go build -o ../dist/plugin.wasm main.go
cd ..

# Copy manifest
echo "Copying manifest..."
cp manifest.json dist/

# Create plugin zip with only the necessary files
echo "Creating plugin package..."
cd dist

# List files that will be included
echo "Files to package:"
ls -la assets/

# Create zip with only the required files
zip -r ../react-demo.zip manifest.json plugin.wasm assets/index.html assets/index-*.js assets/index-*.css

cd ..

echo ""
echo "✓ Plugin built successfully: react-demo.zip"
echo ""
echo "Plugin contents:"
unzip -l react-demo.zip
echo ""
echo "To install manually:"
echo "  1. Extract the zip to your plugins directory:"
echo "     unzip -o react-demo.zip -d ../../plugins/react-demo/"
echo ""
echo "Or to upload via API:"
echo "  1. Start the server: ./windshift"
echo "  2. Upload plugin: curl -X POST -F 'plugin=@plugin-src/react-demo/react-demo.zip' http://localhost:8080/api/plugins/upload"
