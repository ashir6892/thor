#!/data/data/com.termux/files/usr/bin/bash
# Brain Loop Heartbeat - Thor autonomous improvement cycle
# Checks BRAIN_LOOP.md - if Edison just finished, triggers Archimedes for next idea.
# If Archimedes just proposed, triggers Edison to build.
# Runs every 60 seconds via cron. Survives restarts.
# Guardian Prime Safety Overhaul: cycle cap, restart spike abort, manual disable flag

BRAIN_FILE="$HOME/.thor/workspace/memory/BRAIN_LOOP.md"
LOCK_FILE="$HOME/.thor/brain-loop.lock"
LOG_FILE="$HOME/.thor/workspace/memory/brain-loop.log"
DEPLOY_LOCK="$HOME/.thor/brain-loop-deploy.lock"

# Prevent overlapping runs (PID-based lock)
if [ -f "$LOCK_FILE" ]; then
  PID=$(cat "$LOCK_FILE")
  if kill -0 "$PID" 2>/dev/null; then
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Brain loop already running (PID $PID), skipping." >> "$LOG_FILE"
    exit 0
  fi
fi
echo $$ > "$LOCK_FILE"
trap "rm -f $LOCK_FILE" EXIT

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> "$LOG_FILE"
}

# =============================================================================
# SAFETY GUARD 1: Check for manual disable flag
# Created automatically on restart spike, or manually by operator
# Remove ~/.thor/brain-loop-disabled to re-enable
# =============================================================================
if [ -f "$HOME/.thor/brain-loop-disabled" ]; then
  log "Brain loop manually disabled (brain-loop-disabled flag exists). Remove file to re-enable."
  exit 0
fi

# =============================================================================
# SAFETY GUARD 2: PM2 restart spike detection
# If Thor has restarted more than 5 times, disable brain loop and alert
# =============================================================================
RESTART_COUNT=$(pm2 jlist 2>/dev/null | python3 -c "
import sys,json
try:
    procs=json.load(sys.stdin)
    thor=[p for p in procs if p['name']=='thor']
    print(thor[0]['pm2_env']['restart_time'] if thor else 0)
except: print(0)
" 2>/dev/null || echo 0)

if [ "$RESTART_COUNT" -gt 5 ]; then
  log "SAFETY ABORT: Thor has restarted $RESTART_COUNT times - disabling brain loop until manual reset"
  TELEGRAM_TOKEN=$(cat ~/.thor/config.yml 2>/dev/null | grep telegram_token | awk '{print $2}' | tr -d '"')
  curl -s -X POST "https://api.telegram.org/bot${TELEGRAM_TOKEN}/sendMessage" \
    -d "chat_id=1930168837&text=Brain loop SAFETY ABORT: Thor restarted ${RESTART_COUNT} times. Brain loop disabled. Run: ~/thor/scripts/brain-loop-reset.sh to re-enable." > /dev/null
  touch "$HOME/.thor/brain-loop-disabled"
  exit 1
fi

# =============================================================================
# SAFETY GUARD 3: Deploy lock - prevent multiple Edison agents deploying simultaneously
# If deploy lock is less than 600s old, skip this run entirely
# =============================================================================
if [ -f "$DEPLOY_LOCK" ]; then
  AGE=$(( $(date +%s) - $(stat -c %Y "$DEPLOY_LOCK" 2>/dev/null || echo 0) ))
  if [ "$AGE" -lt 600 ]; then
    log "[brain-loop] Deploy lock active ($AGE s old), skipping."
    exit 0
  else
    log "[brain-loop] Deploy lock stale ($AGE s), removing."
    rm -f "$DEPLOY_LOCK"
  fi
fi

# =============================================================================
# SAFETY GUARD 4: Max daily Edison cycles cap
# Prevents runaway loop from triggering Edison more than 5 times per day
# =============================================================================
CYCLE_COUNT_FILE="$HOME/.thor/brain-cycle-count-$(date +%Y%m%d)"
CYCLE_COUNT=$(cat "$CYCLE_COUNT_FILE" 2>/dev/null || echo 0)
MAX_CYCLES=5

if [ "$CYCLE_COUNT" -ge "$MAX_CYCLES" ]; then
  log "Max daily cycles ($MAX_CYCLES) reached - brain loop paused for today"
  exit 0
fi

log "Brain loop heartbeat tick (restart_count=$RESTART_COUNT, cycles_today=$CYCLE_COUNT)"

# Read current state from BRAIN_LOOP.md
LAST_STATE=$(grep "^## BRAIN_LOOP_STATE:" "$BRAIN_FILE" 2>/dev/null | tail -1 | awk '{print $3}')

if [ -z "$LAST_STATE" ]; then
  LAST_STATE="idle"
fi

log "Current state: $LAST_STATE"

case "$LAST_STATE" in
  "idle"|"edison_done")
    # Archimedes turn - propose/pick next feature
    log "Triggering Archimedes (Visionary) cycle"
    # Update state
    sed -i 's/^## BRAIN_LOOP_STATE:.*/## BRAIN_LOOP_STATE: archimedes_running/' "$BRAIN_FILE" 2>/dev/null || \
      echo "## BRAIN_LOOP_STATE: archimedes_running" >> "$BRAIN_FILE"

    THOR_BIN="$HOME/thor/build/thor"
    if [ -f "$THOR_BIN" ]; then
      "$THOR_BIN" -message "ARCHIMEDES CYCLE: Read BRAIN_LOOP.md at ~/.thor/workspace/memory/BRAIN_LOOP.md. Pick the highest priority PENDING idea. Write a detailed implementation plan. Update BRAIN_LOOP.md: mark the chosen idea status as 'archimedes_selected', write your debate note with WHY this idea, HOW to implement it (Go code plan), what files to change. Then set BRAIN_LOOP_STATE to 'archimedes_done'. Sign your note as Archimedes." 2>> "$LOG_FILE"
    fi
    ;;

  "archimedes_done")
    # Edison turn - implement the selected feature
    log "Triggering Edison (Builder) cycle"
    sed -i 's/^## BRAIN_LOOP_STATE:.*/## BRAIN_LOOP_STATE: edison_running/' "$BRAIN_FILE" 2>/dev/null || \
      echo "## BRAIN_LOOP_STATE: edison_running" >> "$BRAIN_FILE"

    # Increment cycle count when triggering Edison
    echo $((CYCLE_COUNT + 1)) > "$CYCLE_COUNT_FILE"
    log "Edison cycle count incremented to $((CYCLE_COUNT + 1)) of $MAX_CYCLES"

    THOR_BIN="$HOME/thor/build/thor"
    if [ -f "$THOR_BIN" ]; then
      "$THOR_BIN" -message "EDISON CYCLE: Read BRAIN_LOOP.md at ~/.thor/workspace/memory/BRAIN_LOOP.md. Find the idea marked 'archimedes_selected'. Implement it fully: write Go code, edit the right files in ~/thor/, run 'go test ./...' to verify, then deploy using ~/thor/scripts/safe-deploy.sh. IMPORTANT SAFETY RULES: (1) You MUST use ~/thor/scripts/safe-deploy.sh for all deploys - NEVER deploy manually. (2) After deploying, wait for the health check to complete before updating BRAIN_LOOP_STATE. (3) If safe-deploy.sh fails, set BRAIN_LOOP_STATE to 'idle' and report the failure via Telegram. After successful deploy, update BRAIN_LOOP.md: mark that idea as 'completed' in the queue table AND move it to the Completed Upgrades table. Set BRAIN_LOOP_STATE to 'edison_done'. Send a Telegram message to chat 1930168837 summarizing what was built and how it works. Sign your note as Edison." 2>> "$LOG_FILE"
    fi
    ;;

  "archimedes_running"|"edison_running")
    # Still in progress - check if it's been too long (stuck)
    LAST_UPDATE=$(grep "^## BRAIN_LOOP_UPDATED:" "$BRAIN_FILE" 2>/dev/null | tail -1 | sed 's/## BRAIN_LOOP_UPDATED: //')
    if [ -n "$LAST_UPDATE" ]; then
      LAST_TS=$(date -d "$LAST_UPDATE" +%s 2>/dev/null || echo 0)
      NOW_TS=$(date +%s)
      DIFF=$((NOW_TS - LAST_TS))
      if [ "$DIFF" -gt 300 ]; then
        # Stuck for more than 5 min - reset to idle
        log "Agent appears stuck for ${DIFF}s (threshold 300s) - resetting state to idle"
        sed -i 's/^## BRAIN_LOOP_STATE:.*/## BRAIN_LOOP_STATE: idle/' "$BRAIN_FILE"
      else
        log "Agent still running (${DIFF}s elapsed, timeout at 300s) - waiting"
      fi
    else
      log "No timestamp found - waiting"
    fi
    ;;

  *)
    log "Unknown state '$LAST_STATE' - resetting to idle"
    sed -i 's/^## BRAIN_LOOP_STATE:.*/## BRAIN_LOOP_STATE: idle/' "$BRAIN_FILE" 2>/dev/null || \
      echo "## BRAIN_LOOP_STATE: idle" >> "$BRAIN_FILE"
    ;;
esac

# Update heartbeat timestamp
sed -i 's/^## BRAIN_LOOP_UPDATED:.*//' "$BRAIN_FILE"
echo "## BRAIN_LOOP_UPDATED: $(date '+%Y-%m-%d %H:%M:%S')" >> "$BRAIN_FILE"

log "Heartbeat tick done. State was: $LAST_STATE"
