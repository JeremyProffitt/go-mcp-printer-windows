#!/bin/bash

# Build script for go-mcp-printer-windows
# Builds for Windows amd64 and arm64

set -e

APP_NAME="go-mcp-printer-windows"
VERSION="${VERSION:-1.0.0}"
BUILD_DIR="dist"

echo "Building $APP_NAME v$VERSION..."

# Clean build directory
rm -rf $BUILD_DIR
mkdir -p $BUILD_DIR

# Generate icon files
echo "Generating icon..."
go run cmd/genicon/main.go

# Build for Windows platforms
PLATFORMS=(
    "windows/amd64"
    "windows/arm64"
)

for PLATFORM in "${PLATFORMS[@]}"; do
    GOOS=${PLATFORM%/*}
    GOARCH=${PLATFORM#*/}
    OUTPUT_NAME="${APP_NAME}-${GOARCH}.exe"

    echo "Building for $GOOS/$GOARCH..."

    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build \
        -ldflags="-s -w -X main.version=$VERSION" \
        -o "$BUILD_DIR/$OUTPUT_NAME" .

    echo "  -> $BUILD_DIR/$OUTPUT_NAME"
done

# Build MSI installer (requires WiX v4+: dotnet tool install --global wix)
if command -v wix &> /dev/null; then
    echo ""
    echo "Building MSI installer..."
    wix build wix/Product.wxs \
        -o "$BUILD_DIR/$APP_NAME.msi" \
        -b BuildDir=$BUILD_DIR \
        -b wix \
        -d Version=$VERSION \
        -ext WixToolset.Firewall.wixext
    echo "  -> $BUILD_DIR/$APP_NAME.msi"
else
    echo ""
    echo "Skipping MSI build (WiX not installed). Install with: dotnet tool install --global wix"
fi

echo ""
echo "Build complete! Output in $BUILD_DIR/"
ls -la $BUILD_DIR/
