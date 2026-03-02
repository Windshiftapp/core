#!/bin/sh
set -e

echo "================================"
echo "  Windshift Demo Server"
echo "================================"
echo ""
echo "Hostname: ${HOSTNAME}"
echo "Internal port: ${WINDSHIFT_PORT:-8080}"
echo ""

# Generate Caddyfile from template
echo "Generating Caddy configuration..."
envsubst '${HOSTNAME}' < /etc/caddy/Caddyfile.template > /etc/caddy/Caddyfile

# Ensure data directory permissions
chown -R windshift:windshift /data 2>/dev/null || true

# Start windshift in background as non-root user
echo "Starting Windshift server..."
su-exec windshift /usr/local/bin/windshift \
    -db /data/demo.db \
    -p ${WINDSHIFT_PORT:-8080} \
    --allowed-hosts "${HOSTNAME},localhost" \
    --allowed-port "80,443,8443" \
    --attachment-path /data/attachments &

WINDSHIFT_PID=$!

# Wait for windshift to be ready
echo "Waiting for Windshift to be ready..."
sleep 2
until wget -q --spider http://localhost:${WINDSHIFT_PORT:-8080}/api/setup/status 2>/dev/null; do
    echo "  Waiting..."
    sleep 1
done

echo ""
echo "Windshift is ready!"
echo ""
echo "Starting Caddy reverse proxy..."
echo ""
echo "================================"
echo "  Demo is starting..."
echo ""
echo "  Main URL:  https://${HOSTNAME}"
echo "  Alt URL:   https://${HOSTNAME}:8443"
echo ""
echo "  Login:     admin / admin"
echo "================================"
echo ""

# Handle shutdown gracefully
trap 'echo "Shutting down..."; kill $WINDSHIFT_PID 2>/dev/null; exit 0' SIGTERM SIGINT

# Start Caddy (foreground) - this keeps the container running
exec caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
