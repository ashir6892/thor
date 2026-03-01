#!/bin/bash
# daily-summary.sh — Thor Daily Digest
# Runs at midnight, sends a Telegram summary of the day's activity.

set -euo pipefail

HOME_DIR="/data/data/com.termux/files/home"
CONFIG="$HOME_DIR/.thor/config.json"
METRICS_LOG="$HOME_DIR/.thor/metrics/response_times.log"
TODAY=$(date +%Y%m%d)
MONTH=$(date +%Y%m)
DAILY_NOTE="$HOME_DIR/.thor/workspace/memory/$MONTH/$TODAY.md"
DATE_DISPLAY=$(date +%Y-%m-%d)

# ── 1. Read Telegram token ─────────────────────────────────────────────────────
TOKEN=$(python3 -c "import json; c=json.load(open('$CONFIG')); print(c['channels']['telegram']['token'])")
CHAT_ID="1930168837"

# ── 2. Metrics ─────────────────────────────────────────────────────────────────
TOTAL_REQUESTS=0
AVG_RESPONSE="N/A"
SLOWEST="N/A"
TOTAL_TOKENS=0

if [[ -f "$METRICS_LOG" ]]; then
    TODAY_ISO=$(date +%Y-%m-%d)
    TODAY_LINES=$(grep "^$TODAY_ISO" "$METRICS_LOG" 2>/dev/null || true)

    if [[ -n "$TODAY_LINES" ]]; then
        TOTAL_REQUESTS=$(echo "$TODAY_LINES" | wc -l | tr -d ' ')

        DURATIONS=$(echo "$TODAY_LINES" | grep -oP 'duration=\K[0-9.]+' || true)
        if [[ -n "$DURATIONS" ]]; then
            AVG_RESPONSE=$(echo "$DURATIONS" | awk '{s+=$1; c++} END {printf "%.1f", s/c}')
            SLOWEST=$(echo "$DURATIONS" | awk 'BEGIN{m=0} {if($1+0>m+0) m=$1} END{printf "%.1f", m}')
        fi

        TOTAL_TOKENS=$(echo "$TODAY_LINES" | grep -oP 'tokens=\K[0-9]+' | awk '{s+=$1} END{print s+0}')
    fi
fi

TOKENS_FMT=$(python3 -c "print('{:,}'.format($TOTAL_TOKENS))")

# ── 3. PM2 stats ───────────────────────────────────────────────────────────────
PM2_UPTIME="unknown"
PM2_RESTARTS="?"
PM2_MEM="?"

PM2_JSON=$(pm2 jlist 2>/dev/null || echo "[]")
if [[ "$PM2_JSON" != "[]" ]]; then
    PM2_RESTARTS=$(echo "$PM2_JSON" | python3 -c "
import json, sys
procs = json.load(sys.stdin)
thor = next((p for p in procs if p.get('name') == 'thor'), None)
if thor:
    print(thor.get('pm2_env', {}).get('restart_time', '?'))
else:
    print('?')
" 2>/dev/null || echo "?")

    PM2_MEM=$(echo "$PM2_JSON" | python3 -c "
import json, sys
procs = json.load(sys.stdin)
thor = next((p for p in procs if p.get('name') == 'thor'), None)
if thor:
    mem = thor.get('monit', {}).get('memory', 0)
    print(str(mem // (1024*1024)) + 'MB')
else:
    print('?')
" 2>/dev/null || echo "?")

    PM2_UPTIME=$(echo "$PM2_JSON" | python3 -c "
import json, sys, time
procs = json.load(sys.stdin)
thor = next((p for p in procs if p.get('name') == 'thor'), None)
if thor:
    pm2_uptime = thor.get('pm2_env', {}).get('pm_uptime', 0)
    now_ms = int(time.time() * 1000)
    elapsed = (now_ms - pm2_uptime) // 1000
    h = elapsed // 3600
    m = (elapsed % 3600) // 60
    print(str(h) + 'h ' + str(m) + 'm')
else:
    print('unknown')
" 2>/dev/null || echo "unknown")
fi

# ── 4. Daily note preview ──────────────────────────────────────────────────────
NOTE_SECTION=""
if [[ -f "$DAILY_NOTE" ]]; then
    NOTE_TEXT=$(head -c 300 "$DAILY_NOTE" | tr '\n' ' ')
    NOTE_SECTION=$(printf "\n\xF0\x9F\x93\x9D *Notes Preview:*\n%s..." "$NOTE_TEXT")
fi

# ── 5. Build and send message via Python (avoids shell quoting nightmares) ─────
python3 << PYEOF
import json, urllib.request, urllib.parse, os

token = open('/dev/stdin').read().strip() if False else None
with open('$CONFIG') as f:
    token = json.load(f)['channels']['telegram']['token']

date_display = "$DATE_DISPLAY"
total_requests = "$TOTAL_REQUESTS"
avg_response = "$AVG_RESPONSE"
slowest = "$SLOWEST"
tokens_fmt = "$TOKENS_FMT"
pm2_restarts = "$PM2_RESTARTS"
pm2_uptime = "$PM2_UPTIME"
pm2_mem = "$PM2_MEM"
note_section = """$NOTE_SECTION"""

msg = (
    f"📊 *Thor Daily Summary — {date_display}*\n\n"
    f"💬 *Requests today:* {total_requests}\n"
    f"⚡ *Avg response:* {avg_response}s | Slowest: {slowest}s\n"
    f"🪙 *Tokens used:* {tokens_fmt}\n"
    f"🔄 *Restarts:* {pm2_restarts} | Uptime: {pm2_uptime}\n"
    f"💾 *Memory:* {pm2_mem}"
    f"{note_section}\n\n"
    f"🦞 Thor is watching over you!"
)

payload = json.dumps({
    "chat_id": "1930168837",
    "parse_mode": "Markdown",
    "text": msg
}).encode()

req = urllib.request.Request(
    f"https://api.telegram.org/bot{token}/sendMessage",
    data=payload,
    headers={"Content-Type": "application/json"}
)
with urllib.request.urlopen(req, timeout=15) as resp:
    result = json.load(resp)
    if result.get("ok"):
        print("OK: message_id=" + str(result["result"]["message_id"]))
    else:
        print("ERROR: " + str(result))
        exit(1)
PYEOF

echo "[$(date -Iseconds)] Daily summary sent. Requests=$TOTAL_REQUESTS Tokens=$TOTAL_TOKENS"
