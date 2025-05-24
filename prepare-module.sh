#!/bin/bash

echo "=== Preparing UDP Network as Go Module ==="
echo ""

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Current module name:${NC} $(grep "^module" go.mod)"
echo ""

echo "To use this as a module in other projects:"
echo ""

echo -e "${GREEN}1. Update module name:${NC}"
echo "   Edit go.mod and change the module name to your repository:"
echo "   module github.com/yourusername/udp_network"
echo ""

echo -e "${GREEN}2. Update imports:${NC}"
echo "   Run this command to update all imports:"
echo "   find . -name '*.go' -type f -exec sed -i 's|github.com/yourusername/udp_network|github.com/YOUR_USERNAME/udp_network|g' {} +"
echo ""

echo -e "${GREEN}3. Initialize git (if not done):${NC}"
echo "   git init"
echo "   git add ."
echo "   git commit -m \"Initial commit\""
echo ""

echo -e "${GREEN}4. Push to GitHub:${NC}"
echo "   git remote add origin https://github.com/YOUR_USERNAME/udp_network.git"
echo "   git push -u origin main"
echo ""

echo -e "${GREEN}5. Tag a version:${NC}"
echo "   git tag v1.0.0"
echo "   git push origin v1.0.0"
echo ""

echo -e "${GREEN}6. Use in your project:${NC}"
echo "   go get github.com/YOUR_USERNAME/udp_network@v1.0.0"
echo ""

echo -e "${YELLOW}For local development without GitHub:${NC}"
echo "   See example-project/go.mod for how to use 'replace' directive"
echo ""

# Check if everything builds
echo -e "${GREEN}Checking build...${NC}"
if go build ./... 2>/dev/null; then
    echo -e "${GREEN}✓ Module builds successfully${NC}"
else
    echo -e "${YELLOW}⚠ Build errors detected. Fix these before publishing.${NC}"
fi

echo ""
echo "Module structure:"
echo "  Main package: github.com/yourusername/udp_network"
echo "  Protocol:     github.com/yourusername/udp_network/protocol"
echo "  Tokenizer:    github.com/yourusername/udp_network/tokenizer"