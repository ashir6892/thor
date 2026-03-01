#!/bin/bash
# Thor Metrics Dashboard
# Shows performance stats, uptime, and recent activity

echo "=== Thor Metrics Dashboard ==="
echo "Generated: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

echo "📊 Response Times (last 20):"
tail -20 ~/.thor/metrics/response_times.log 2>/dev/null || echo "  No data yet"
echo ""

echo "⏱️  Average Response Time:"
if [ -f ~/.thor/metrics/response_times.log ] && [ -s ~/.thor/metrics/response_times.log ]; then
    awk '{
        for(i=1;i<=NF;i++) {
            if($i ~ /^duration=/) {
                val = $i
                sub("duration=", "", val)
                sub("s$", "", val)
                sum += val
                count++
            }
        }
    } END {
        if(count > 0)
            printf "  %.2fs avg over %d requests\n", sum/count, count
        else
            print "  No data"
    }' ~/.thor/metrics/response_times.log
else
    echo "  No data yet"
fi
echo ""

echo "📈 Requests Today:"
TODAY=$(date '+%Y-%m-%d')
if [ -f ~/.thor/metrics/response_times.log ]; then
    COUNT=$(grep "$TODAY" ~/.thor/metrics/response_times.log 2>/dev/null | wc -l)
    echo "  $COUNT requests on $TODAY"
else
    echo "  No data yet"
fi
echo ""

echo "⏳ Uptime Log (last 10):"
tail -10 ~/.thor/metrics/uptime.log 2>/dev/null || echo "  No uptime data yet"
echo ""

echo "🔄 PM2 Status:"
pm2 list 2>/dev/null || echo "  PM2 not available"
echo ""

echo "📝 Recent Git Log:"
cd /data/data/com.termux/files/home/thor && git log --oneline -10 2>/dev/null || echo "  Git not available"
echo ""

echo "🧠 Memory Usage:"
if command -v pm2 &>/dev/null; then
    pm2 jlist 2>/dev/null | python3 -c "
import sys, json
try:
    procs = json.load(sys.stdin)
    for p in procs:
        if p['name'] == 'thor':
            mem = p.get('monit', {}).get('memory', 0)
            cpu = p.get('monit', {}).get('cpu', 0)
            restarts = p.get('pm2_env', {}).get('restart_time', 0)
            status = p.get('pm2_env', {}).get('status', 'unknown')
            print(f'  Status: {status}')
            print(f'  Memory: {mem // 1024 // 1024}MB')
            print(f'  CPU: {cpu}%')
            print(f'  Restarts: {restarts}')
except Exception as e:
    print(f'  Error reading PM2 data: {e}')
" 2>/dev/null || echo "  Could not read PM2 stats"
fi
echo ""

echo "📂 Metrics Files:"
ls -lh ~/.thor/metrics/ 2>/dev/null || echo "  No metrics directory"
echo ""
echo "=== End Dashboard ==="
