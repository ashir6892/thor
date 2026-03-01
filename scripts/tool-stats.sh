#!/bin/sh
# Thor Tool Analytics Dashboard
# Shows per-tool call counts, avg latency, error rates from ~/.thor/metrics/tool_analytics.jsonl

LOGFILE="$HOME/.thor/metrics/tool_analytics.jsonl"

echo "=== 🔧 Thor Tool Analytics ==="
echo "Generated: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

if [ ! -f "$LOGFILE" ] || [ ! -s "$LOGFILE" ]; then
    echo "No tool analytics data yet. Data is collected as tools are used."
    exit 0
fi

TOTAL=$(wc -l < "$LOGFILE")
echo "📊 Total tool calls recorded: $TOTAL"
echo ""

python3 - "$LOGFILE" <<'PYEOF'
import sys, json, collections

logfile = sys.argv[1]

calls = collections.defaultdict(list)
errors = collections.defaultdict(int)

with open(logfile) as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        try:
            entry = json.loads(line)
            tool = entry.get("tool", "unknown")
            calls[tool].append(entry.get("duration_ms", 0))
            if entry.get("error"):
                errors[tool] += 1
        except Exception:
            continue

# Sort by call count desc
sorted_tools = sorted(calls.items(), key=lambda x: len(x[1]), reverse=True)

print(f"{'Tool':<30} {'Calls':>6} {'Avg ms':>8} {'Max ms':>8} {'Errors':>7} {'Err%':>6}")
print("-" * 70)
for tool, durations in sorted_tools:
    count = len(durations)
    avg_ms = sum(durations) / count if count else 0
    max_ms = max(durations) if durations else 0
    err_count = errors.get(tool, 0)
    err_pct = (err_count / count * 100) if count else 0
    print(f"{tool:<30} {count:>6} {avg_ms:>8.0f} {max_ms:>8} {err_count:>7} {err_pct:>5.1f}%")

print()
# Top 3 slowest tools
slowest = sorted(calls.items(), key=lambda x: max(x[1]) if x[1] else 0, reverse=True)[:3]
print("⏱️  Top 3 Slowest Tools (by max latency):")
for tool, durations in slowest:
    print(f"  {tool}: {max(durations)}ms max")

print()
# Most errored
most_errored = sorted(errors.items(), key=lambda x: x[1], reverse=True)[:3]
if most_errored:
    print("⚠️  Top 3 Most Errored Tools:")
    for tool, count in most_errored:
        print(f"  {tool}: {count} errors")
PYEOF

echo ""
echo "📁 Log: $LOGFILE"
echo "=== End Analytics ==="
