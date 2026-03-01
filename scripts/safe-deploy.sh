#!/bin/bash
# Safe deploy script - builds new binary, tests it, deploys with auto-restore on failure

BINARY="/data/data/com.termux/files/home/thor/build/thor"
BACKUP="/data/data/com.termux/files/home/thor/build/thor.backup"
SOURCE="/data/data/com.termux/files/home/thor"
TELEGRAM_TOKEN=$(cat ~/.thor/config.yml | grep telegram_token | awk '{print $2}' | tr -d '"')
CHAT_ID="1930168837"

send_telegram() {
    curl -s -X POST "https://api.telegram.org/bot${TELEGRAM_TOKEN}/sendMessage" \
        -d "chat_id=${CHAT_ID}&text=$1" > /dev/null
}

echo "🔨 Building Thor..."
cd $SOURCE
go build -o build/thor.new ./cmd/thor/

if [ $? -ne 0 ]; then
    send_telegram "❌ Thor build FAILED — staying on current binary. Check logs!"
    exit 1
fi

echo "✅ Build succeeded. Backing up current binary..."
cp $BINARY $BACKUP

echo "🚀 Deploying new binary..."
cp build/thor.new $BINARY
rm build/thor.new

echo "🔄 Restarting Thor via PM2..."
pm2 restart thor

echo "⏳ Waiting 15 seconds to verify Thor is healthy..."
sleep 15

# Check if Thor is still running
STATUS=$(pm2 jlist | python3 -c "import sys,json; procs=json.load(sys.stdin); thor=[p for p in procs if p['name']=='thor']; print(thor[0]['pm2_env']['status'] if thor else 'not_found')" 2>/dev/null)

if [ "$STATUS" = "online" ]; then
    send_telegram "✅ Thor deployed successfully! New binary is live and healthy. 🎉"
    echo "✅ Deploy successful!"
else
    echo "❌ Thor is not healthy (status: $STATUS). Restoring backup..."
    send_telegram "⚠️ New Thor binary failed health check (status: $STATUS)! Auto-restoring previous version..."
    
    cp $BACKUP $BINARY
    pm2 restart thor
    sleep 10
    
    STATUS2=$(pm2 jlist | python3 -c "import sys,json; procs=json.load(sys.stdin); thor=[p for p in procs if p['name']=='thor']; print(thor[0]['pm2_env']['status'] if thor else 'not_found')" 2>/dev/null)
    
    if [ "$STATUS2" = "online" ]; then
        send_telegram "✅ Previous Thor version restored successfully! Check logs for what went wrong."
    else
        send_telegram "🚨 CRITICAL: Both new and backup Thor failed! Manual intervention needed. Run: pm2 logs thor"
    fi
    exit 1
fi
