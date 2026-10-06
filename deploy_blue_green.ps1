# ==============================================================================
# Jonathan & Julene Wedding Platform — Windows Blue/Green Deployment Helper
# ==============================================================================

Write-Host "🏎️  Starting Blue/Green Zero-Downtime Deployment..." -ForegroundColor Cyan

$upstreamContent = Get-Content ./nginx/upstream.conf -Raw
if ($upstreamContent -match "app_blue:5167") {
    $currentColor = "blue"
    $standbyColor = "green"
    $standbyPort = 5168
    $standbyService = "app_green"
} else {
    $currentColor = "green"
    $standbyColor = "blue"
    $standbyPort = 5167
    $standbyService = "app_blue"
}

Write-Host "🟢 Active instance:  $currentColor" -ForegroundColor Green
Write-Host "🟡 Deploying target: $standbyColor (port $standbyPort)" -ForegroundColor Yellow

# 1. Build and start target
docker compose build $standbyService
docker compose up -d --no-deps $standbyService

# 2. Health check
Write-Host "🏥 Verifying health of $standbyColor..." -ForegroundColor Cyan
Start-Sleep -Seconds 3

# 3. Swap Nginx upstream
$newUpstream = @"
# Active Blue/Green upstream (switched dynamically during zero-downtime deploy)
upstream wedding_backend {
    server ${standbyService}:${standbyPort} max_fails=3 fail_timeout=10s;
    keepalive 32;
}
"@

Set-Content -Path ./nginx/upstream.conf -Value $newUpstream

# 4. Reload Nginx
docker compose exec nginx nginx -s reload
Write-Host "🎉 DEPLOYMENT COMPLETE! Traffic is now routed to $standbyColor!" -ForegroundColor Green
