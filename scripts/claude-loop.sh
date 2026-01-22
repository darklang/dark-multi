#!/bin/bash
# claude-loop.sh - Ralph Wiggum style loop for Claude
# Runs Claude repeatedly until completion or max iterations

set -e

TASK_DIR="/home/dark/app/.claude-task"
PHASE_FILE="$TASK_DIR/phase"
STATUS_FILE="$TASK_DIR/status.md"
MAX_ITERATIONS=${MAX_ITERATIONS:-100}
ITERATION=0

# Ensure task directory exists
mkdir -p "$TASK_DIR"

# Initialize phase file if not exists
if [ ! -f "$PHASE_FILE" ]; then
    echo "executing" > "$PHASE_FILE"
fi

log() {
    echo "[claude-loop] $1"
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" >> "$TASK_DIR/loop.log"
}

update_status() {
    echo "# Loop Status" > "$STATUS_FILE"
    echo "" >> "$STATUS_FILE"
    echo "**Iteration:** $ITERATION / $MAX_ITERATIONS" >> "$STATUS_FILE"
    echo "**Phase:** $(cat $PHASE_FILE 2>/dev/null || echo 'unknown')" >> "$STATUS_FILE"
    echo "**Last update:** $(date '+%Y-%m-%d %H:%M:%S')" >> "$STATUS_FILE"
    echo "" >> "$STATUS_FILE"
    echo "## Recent Activity" >> "$STATUS_FILE"
    echo "" >> "$STATUS_FILE"
}

check_phase_transition() {
    # Check if Claude output contains phase transition signals
    local output_file="$1"

    if grep -q "<phase>AWAITING_ANSWERS</phase>" "$output_file" 2>/dev/null; then
        echo "awaiting-answers" > "$PHASE_FILE"
        log "Phase transition: awaiting-answers"
        return 0  # Stop loop
    fi

    if grep -q "<phase>READY_TO_EXECUTE</phase>" "$output_file" 2>/dev/null; then
        echo "executing" > "$PHASE_FILE"
        log "Phase transition: executing"
        return 1  # Continue loop
    fi

    if grep -q "<phase>READY_FOR_REVIEW</phase>" "$output_file" 2>/dev/null; then
        echo "ready-for-review" > "$PHASE_FILE"
        log "Phase transition: ready-for-review"
        return 0  # Stop loop
    fi

    if grep -q "<phase>NEEDS_HELP</phase>" "$output_file" 2>/dev/null; then
        echo "awaiting-answers" > "$PHASE_FILE"
        log "Phase transition: needs-help -> awaiting-answers"
        return 0  # Stop loop
    fi

    if grep -q "<phase>CLEANUP</phase>" "$output_file" 2>/dev/null; then
        echo "cleanup" > "$PHASE_FILE"
        log "Phase transition: cleanup"
        return 1  # Continue with cleanup
    fi

    if grep -q "<phase>DONE</phase>" "$output_file" 2>/dev/null; then
        echo "done" > "$PHASE_FILE"
        log "Phase transition: done"
        return 0  # Stop loop
    fi

    return 1  # No transition, continue if in executing phase
}

run_claude() {
    local output_file="$TASK_DIR/claude-output-$ITERATION.log"

    log "Starting Claude iteration $ITERATION"
    update_status

    # Run Claude, capturing output
    # Note: --dangerously-skip-permissions allows autonomous operation
    if command -v claude &> /dev/null; then
        # Tee output to both terminal and file for phase detection
        claude --dangerously-skip-permissions 2>&1 | tee "$output_file"
        local exit_code=${PIPESTATUS[0]}
    else
        log "ERROR: claude command not found"
        return 1
    fi

    log "Claude exited with code $exit_code"

    # Check for phase transitions in output
    if check_phase_transition "$output_file"; then
        return 0  # Phase transition requires stopping
    fi

    # Clean up old output files (keep last 5)
    ls -t "$TASK_DIR"/claude-output-*.log 2>/dev/null | tail -n +6 | xargs rm -f 2>/dev/null || true

    return $exit_code
}

main() {
    log "Starting Claude loop (max $MAX_ITERATIONS iterations)"

    while [ $ITERATION -lt $MAX_ITERATIONS ]; do
        ITERATION=$((ITERATION + 1))

        current_phase=$(cat "$PHASE_FILE" 2>/dev/null || echo "executing")

        # Only loop in executing or cleanup phases
        if [ "$current_phase" != "executing" ] && [ "$current_phase" != "cleanup" ] && [ "$current_phase" != "planning" ]; then
            log "Phase is $current_phase, stopping loop"
            break
        fi

        if ! run_claude; then
            # Claude exited, check if we should continue
            current_phase=$(cat "$PHASE_FILE" 2>/dev/null || echo "unknown")

            if [ "$current_phase" = "executing" ]; then
                log "Claude exited during executing phase, restarting in 2 seconds..."
                sleep 2
            else
                log "Loop complete (phase: $current_phase)"
                break
            fi
        else
            # Phase transition occurred
            break
        fi
    done

    if [ $ITERATION -ge $MAX_ITERATIONS ]; then
        log "WARNING: Max iterations ($MAX_ITERATIONS) reached"
        echo "max-iterations-reached" > "$PHASE_FILE"
    fi

    log "Loop finished after $ITERATION iterations"
}

# Run main if not sourced
if [ "${BASH_SOURCE[0]}" == "${0}" ]; then
    main "$@"
fi
