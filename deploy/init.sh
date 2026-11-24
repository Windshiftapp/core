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

# Strict trust with no proxy source would attribute every client to the proxy.
if grep -q "^PROXY_TRUST_MODE=strict" .env 2>/dev/null && grep -q "^ADDITIONAL_PROXIES=$" .env 2>/dev/null; then
    echo "" >&2
    echo "ACTION REQUIRED: ADDITIONAL_PROXIES is empty while strict proxy trust is on." >&2
    echo "Set it to the address your reverse proxy uses to reach Windshift (found on" >&2
    echo "Admin -> Diagnostics -> Proxy trust), or leave it empty only for a loopback" >&2
    echo "sidecar. Until then, clients are attributed to the proxy address." >&2
fi

echo ""
echo "Configuration complete!"
echo "Next steps:"
echo "  1. Edit .env and set DOMAIN to your hostname"
echo "  2. Set ADDITIONAL_PROXIES in .env to the address your reverse proxy uses"
echo "     to reach Windshift (leave it empty only for a loopback sidecar)."
echo "  3. Run: docker compose up -d"
echo "  4. Open Admin -> Diagnostics -> Proxy trust, confirm Status is 'protected'"
echo "     and 'Unlisted proxies' is empty, and correct ADDITIONAL_PROXIES if not."
echo ""
