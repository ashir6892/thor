#!/bin/bash
# brain-loop-reset.sh - Reset brain loop safety flags
# Run this when you want to re-enable the brain loop after a safety abort
# Guardian Prime Safety Overhaul utility

echo "Resetting brain loop safety flags..."

rm -f ~/.thor/brain-loop-disabled
echo "  [OK] Removed brain-loop-disabled flag"

rm -f ~/.thor/brain-loop-deploy.lock
echo "  [OK] Removed brain-loop-deploy.lock"

rm -f ~/.thor/brain-loop.lock
echo "  [OK] Removed brain-loop.lock"

pm2 reset thor
echo "  [OK] PM2 restart counter reset for thor"

echo ""
echo "Done! Brain loop re-enabled. Restart count reset."
echo "The brain loop will resume on its next scheduled cron tick."
