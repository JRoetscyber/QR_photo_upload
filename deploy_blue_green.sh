#!/usr/bin/env bash
set -e

# ==============================================================================
# Jonathan & Julene Wedding Platform — Blue/Green Zero-Downtime Deployment
# ==============================================================================

echo "🏎️  Starting Blue/Green Zero-Downtime Deployment..."

# Determine currently active color by checking upstream.conf
if grep -q "app_blue:5167" ./nginx/upstream.conf; then
    CURRENT_COLOR="blue"
    STANDBY_COLOR="green"
    STANDBY_PORT=5168
    STANDBY_SERVICE="app_green"
else
    CURRENT_COLOR="green"
    STANDBY_COLOR="blue"
    STANDBY_PORT=5167
    STANDBY_SERVICE="app_blue"
fi

echo "🟢 Active instance:  ${CURRENT_COLOR}"
echo "🟡 Deploying target: ${STANDBY_COLOR} (port ${STANDBY_PORT})"

# 1. Build & start the standby container
echo "🔨 Building and starting ${STANDBY_SERVICE}..."
docker compose build ${STANDBY_SERVICE}
docker compose up -d --no-deps ${STANDBY_SERVICE}

# 2. Perform rigorous health check on standby instance
echo "🏥 Verifying health of ${STANDBY_COLOR} on internal port ${STANDBY_PORT}..."
MAX_ATTEMPTS=20
ATTEMPT=1
HEALTHY=false

while [ $ATTEMPT -le $MAX_ATTEMPTS ]; do
    if docker compose exec ${STANDBY_SERVICE} wget -q -O - http://127.0.0.1:${STANDBY_PORT}/api/health | grep -q '"status":"ok"'; then
        HEALTHY=true
        break
    fi
    echo "   Attempt ${ATTEMPT}/${MAX_ATTEMPTS} - waiting for ${STANDBY_COLOR} to be ready..."
    sleep 2
    ATTEMPT=$((ATTEMPT + 1))
done

if [ "$HEALTHY" != "true" ]; then
    echo "❌ ERROR: ${STANDBY_COLOR} failed health check! Aborting switchover to maintain 100% uptime on ${CURRENT_COLOR}."
    exit 1
fi

echo "✅ ${STANDBY_COLOR} is 100% healthy!"

# 3. Hot-swap Nginx upstream to standby color
echo "⚡ Hot-swapping Nginx upstream from ${CURRENT_COLOR} to ${STANDBY_COLOR}..."
cat <<EOF > ./nginx/upstream.conf
# Active Blue/Green upstream (switched dynamically during zero-downtime deploy)
upstream wedding_backend {
    server ${STANDBY_SERVICE}:${STANDBY_PORT} max_fails=3 fail_timeout=10s;
    keepalive 32;
}
EOF

# 4. Zero-downtime Nginx reload
echo "🔄 Reloading Nginx configuration..."
docker compose exec nginx nginx -s reload

echo "🎉 DEPLOYMENT COMPLETE! Traffic is now seamlessly routed to ${STANDBY_COLOR} with 0ms downtime!"
