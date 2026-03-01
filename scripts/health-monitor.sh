#!/bin/bash
# Monitors Thor health every 5 minutes and auto-restores if crashed

BINARY="/data/data/com.termux/files/home/thor/build/thor"
BACKUP="/data/data/com.termux/files/home/thor/build/thor.backup"
TELEGRAM_TOKEN=$(cat ~/.thor/config.yml | grep telegram_token | awk '{print $2}' | tr -d '"')
CHAT_ID="1930168837"

send_telegram() {
    curl -s -X POST "https://api.telegram.org/bot${TELEGRAM_TOKEN}/sendMessage" \
        -d "chat_id=${CHAT_ID}&text=$1" > /dev/null
}

STATUS=$(pm2 jlist 2>/dev/null | python3 -c "import sys,json; procs=json.load(sys.stdin); thor=[p for p in procs if p['name']=='thor']; print(thor[0]['pm2_env']['status'] if thor else 'not_found')" 2>/dev/null)

if [ "$STATUS" != "online" ]; then
    echo "Thor is not online (status: $STATUS). Attempting restore..."
    
    if [ -f "$BACKUP" ]; then
        cp $BACKUP $BINARY
        send_telegram "🔄 Thor health monitor detected crash (status: $STATUS). Restoring backup binary and restarting..."
    fi
    
    pm2 restart thor
    sleep 10
    
    STATUS2=$(pm2 jlist 2>/dev/null | python3 -c "import sys,json; procs=json.load(sys.stdin); thor=[p for p in procs if p['name']=='thor']; print(thor[0]['pm2_env']['status'] if thor else 'not_found')" 2>/dev/null)
    
    if [ "$STATUS2" = "online" ]; then
        send_telegram "✅ Thor auto-restored successfully after crash!"
    else
        send_telegram "🚨 CRITICAL: Thor auto-restore failed! Status: $STATUS2. Manual intervention needed!"
    fi
fi
