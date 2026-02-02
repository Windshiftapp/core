#!/bin/bash
# Windshift initialization script
# Generates .env file with SSO_SECRET for first-time setup

set -e

cd "$(dirname "$0")"

# Create .env from example if it doesn't exist
if [ ! -f .env ]; then
    cp .env.example .env
    echo "Created .env from template"
fi

# Generate SSO_SECRET only if not already set
if grep -q "^SSO_SECRET=$" .env 2>/dev/null; then
    SSO_SECRET=$(openssl rand -hex 32)
    # Use portable sed syntax (works on both Linux and macOS)
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' "s/^SSO_SECRET=$/SSO_SECRET=$SSO_SECRET/" .env
    else
        sed -i "s/^SSO_SECRET=$/SSO_SECRET=$SSO_SECRET/" .env
    fi
    echo "Generated SSO_SECRET"
else
    echo "SSO_SECRET already configured, skipping"
fi

echo ""
echo "Configuration complete!"
echo "Next steps:"
echo "  1. Edit .env and set DOMAIN to your hostname"
echo "  2. Run: docker compose up -d"
echo ""
