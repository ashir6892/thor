#!/bin/bash
# Safe deploy script - builds new binary, tests it, deploys with auto-restore on failure
# Guardian Prime Safety Overhaul: deploy rate limit, restart spike detection, brain loop lock

BINARY="/data/data/com.termux/files/home/thor/build/thor"
BACKUP="/data/data/com.termux/files/home/thor/build/thor.backup"
SOURCE="/data/data/com.termux/files/home/thor"
TELEGRAM_TOKEN=$(cat ~/.thor/config.yml | grep telegram_token | awk '{print $2}' | tr -d '"')
CHAT_ID="1930168837"

# Brain loop deploy lock - prevents brain loop from firing during deploy window
BRAIN_LOOP_DEPLOY_LOCK="$HOME/.thor/brain-loop-deploy.lock"

send_telegram() {
    curl -s -X POST "https://api.telegram.org/bot${TELEGRAM_TOKEN}/sendMessage" \
        -d "chat_id=${CHAT_ID}&text=$1" > /dev/null
}

# =============================================================================
# SAFETY GUARD 1: PM2 restart spike detection
# Refuse to deploy if Thor has restarted more than 10 times - something is wrong
# =============================================================================
RESTART_COUNT=$(pm2 jlist 2>/dev/null | python3 -c "
import sys,json
try:
    procs=json.load(sys.stdin)
    thor=[p for p in procs if p['name']=='thor']
    print(thor[0]['pm2_env']['restart_time'] if thor else 0)
except: print(0)
" 2>/dev/null || echo 0)

if [ "$RESTART_COUNT" -gt 10 ]; then
    send_telegram "Warning: Thor has restarted $RESTART_COUNT times - something is wrong. Refusing to deploy until restarts are below 10. Fix the issue first! Run: pm2 reset thor"
    echo "SAFETY ABORT: Thor restart count ($RESTART_COUNT) exceeds threshold (10). Deploy blocked."
    exit 1
fi

# =============================================================================
# SAFETY GUARD 2: Deploy rate limit - max 3 deploys per day
# =============================================================================
DEPLOY_COUNT_FILE="$HOME/.thor/deploy-count-$(date +%Y%m%d)"
COUNT=$(cat "$DEPLOY_COUNT_FILE" 2>/dev/null || echo 0)
MAX_DEPLOYS=3

if [ "$COUNT" -ge "$MAX_DEPLOYS" ]; then
    send_telegram "Deploy BLOCKED - max $MAX_DEPLOYS deploys/day reached! Manual override required."
    echo "SAFETY ABORT: Daily deploy limit ($MAX_DEPLOYS) reached. Deploy blocked."
    exit 1
fi

# Increment deploy counter immediately (before deploy, to prevent race)
echo $((COUNT + 1)) > "$DEPLOY_COUNT_FILE"
echo "Deploy #$((COUNT + 1)) of $MAX_DEPLOYS today."

# =============================================================================
# SAFETY GUARD 3: Write brain loop deploy lock
# Brain-loop.sh checks this file - if less than 600s old, it skips its cycle
# This prevents Edison re-triggering during the deploy + health check window
# =============================================================================
touch "$BRAIN_LOOP_DEPLOY_LOCK"
echo "Brain loop deploy lock set: $BRAIN_LOOP_DEPLOY_LOCK"

# Ensure lock is removed on exit (success or failure)
cleanup_lock() {
    rm -f "$BRAIN_LOOP_DEPLOY_LOCK"
    echo "Brain loop deploy lock released."
}

echo "Building Thor..."
cd $SOURCE
go build -o build/thor.new ./cmd/thor/

if [ $? -ne 0 ]; then
    send_telegram "Thor build FAILED - staying on current binary. Check logs!"
    cleanup_lock
    exit 1
fi

echo "Build succeeded. Backing up current binary..."
cp $BINARY $BACKUP

echo "Deploying new binary..."
cp build/thor.new $BINARY
rm build/thor.new

echo "Restarting Thor via PM2..."
pm2 restart thor

echo "Waiting 30 seconds to verify Thor is healthy..."
sleep 30

# Check if Thor is still running
STATUS=$(pm2 jlist | python3 -c "import sys,json; procs=json.load(sys.stdin); thor=[p for p in procs if p['name']=='thor']; print(thor[0]['pm2_env']['status'] if thor else 'not_found')" 2>/dev/null)

if [ "$STATUS" = "online" ]; then
    # Save current commit hash as the "last deployed" marker for /gitdiff command
    CURRENT_COMMIT=$(git -C $SOURCE rev-parse --short HEAD 2>/dev/null)
    if [ -n "$CURRENT_COMMIT" ]; then
        mkdir -p ~/.thor
        echo "$CURRENT_COMMIT" > ~/.thor/last_deploy_commit
        echo "Deploy marker saved: $CURRENT_COMMIT"
    fi

    # Reset PM2 restart counter after successful deploy (clean slate)
    pm2 reset thor
    echo "PM2 restart counter reset."

    # Release brain loop lock - deploy succeeded, health check passed
    cleanup_lock

    send_telegram "Thor deployed successfully! New binary is live and healthy."
    echo "Deploy successful!"
else
    echo "Thor is not healthy (status: $STATUS). Restoring backup..."
    send_telegram "New Thor binary failed health check (status: $STATUS)! Auto-restoring previous version..."

    cp $BACKUP $BINARY
    pm2 restart thor
    sleep 10

    STATUS2=$(pm2 jlist | python3 -c "import sys,json; procs=json.load(sys.stdin); thor=[p for p in procs if p['name']=='thor']; print(thor[0]['pm2_env']['status'] if thor else 'not_found')" 2>/dev/null)

    if [ "$STATUS2" = "online" ]; then
        send_telegram "Previous Thor version restored successfully! Check logs for what went wrong."
    else
        send_telegram "CRITICAL: Both new and backup Thor failed! Manual intervention needed. Run: pm2 logs thor"
    fi

    # Release brain loop lock - rollback complete
    cleanup_lock
    exit 1
fi
