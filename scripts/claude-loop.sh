#!/bin/bash
# claude-loop.sh - Ralph Wiggum style loop for Claude
# Runs Claude repeatedly until completion or max iterations
# Supports recovery from auth failures and iteration limits

set -e

TASK_DIR="/home/dark/app/.claude-task"
PHASE_FILE="$TASK_DIR/phase"
STATUS_FILE="$TASK_DIR/status.md"
ERROR_FILE="$TASK_DIR/error"
ITERATION_FILE="$TASK_DIR/iteration"
MAX_ITERATIONS=${MAX_ITERATIONS:-100}

# Load iteration count (allows recovery after restart)
if [ -f "$ITERATION_FILE" ]; then
    ITERATION=$(cat "$ITERATION_FILE")
else
    ITERATION=0
fi

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

# Clear any OAuth tokens to avoid auth conflicts
# We use ANTHROPIC_API_KEY from environment, not OAuth
clear_oauth_tokens() {
    rm -f ~/.claude/.credentials.json ~/.claude/credentials.json ~/.claude/oauth* 2>/dev/null || true
    # Ensure settings use API key from env
    if [ -d ~/.claude ]; then
        echo '{"theme":"dark","hasCompletedOnboarding":true,"apiKeySource":"env","hasAcknowledgedCostThreshold":true}' > ~/.claude/settings.json
    fi
}

update_status() {
    echo "# Loop Status" > "$STATUS_FILE"
    echo "" >> "$STATUS_FILE"
    echo "**Iteration:** $ITERATION / $MAX_ITERATIONS" >> "$STATUS_FILE"
    echo "**Phase:** $(cat $PHASE_FILE 2>/dev/null || echo 'unknown')" >> "$STATUS_FILE"
    echo "**Last update:** $(date '+%Y-%m-%d %H:%M:%S')" >> "$STATUS_FILE"
    if [ -f "$ERROR_FILE" ]; then
        echo "**Error:** $(cat $ERROR_FILE)" >> "$STATUS_FILE"
    fi
    echo "" >> "$STATUS_FILE"
    echo "## Recent Activity" >> "$STATUS_FILE"
    echo "" >> "$STATUS_FILE"
}

save_iteration() {
    echo "$ITERATION" > "$ITERATION_FILE"
}

check_auth_error() {
    local output_file="$1"

    # Check for various auth error patterns
    if grep -qi "invalid.*api.*key\|authentication.*failed\|unauthorized\|401\|api.*key.*invalid\|ANTHROPIC_API_KEY" "$output_file" 2>/dev/null; then
        return 0  # Auth error detected
    fi
    return 1
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
    save_iteration

    # Clear any previous error
    rm -f "$ERROR_FILE"

    # Run Claude, capturing output
    # Note: --dangerously-skip-permissions allows autonomous operation
    if command -v claude &> /dev/null; then
        # Tee output to both terminal and file for phase detection
        claude --dangerously-skip-permissions 2>&1 | tee "$output_file"
        local exit_code=${PIPESTATUS[0]}
    else
        log "ERROR: claude command not found"
        echo "claude command not found" > "$ERROR_FILE"
        return 1
    fi

    log "Claude exited with code $exit_code"

    # Check for auth errors first
    if check_auth_error "$output_file"; then
        log "ERROR: Authentication failure detected"
        echo "auth-error" > "$PHASE_FILE"
        echo "Authentication failed - check ANTHROPIC_API_KEY" > "$ERROR_FILE"
        return 2  # Special exit code for auth error
    fi

    # Check for phase transitions in output
    if check_phase_transition "$output_file"; then
        return 0  # Phase transition requires stopping
    fi

    # Clean up old output files (keep last 5)
    ls -t "$TASK_DIR"/claude-output-*.log 2>/dev/null | tail -n +6 | xargs rm -f 2>/dev/null || true

    return $exit_code
}

reset_loop() {
    log "Resetting loop state"
    ITERATION=0
    save_iteration
    rm -f "$ERROR_FILE"
    echo "executing" > "$PHASE_FILE"
}

main() {
    # Clear any OAuth tokens to avoid auth conflicts
    clear_oauth_tokens

    # Check for reset flag
    if [ "$1" = "--reset" ] || [ "$1" = "-r" ]; then
        reset_loop
    fi

    # Check current phase - if it's an error state, wait for manual intervention
    current_phase=$(cat "$PHASE_FILE" 2>/dev/null || echo "executing")
    if [ "$current_phase" = "auth-error" ]; then
        log "Auth error state - waiting for fix. Run with --reset after fixing API key."
        echo "Waiting for auth fix. Set ANTHROPIC_API_KEY and run: .claude-task/ralph.sh --reset"
        sleep 30
        exit 1
    fi

    # Check if we hit max iterations previously
    if [ "$current_phase" = "max-iterations-reached" ]; then
        log "Max iterations was reached. Run with --reset to continue."
        echo "Max iterations reached. Run: .claude-task/ralph.sh --reset"
        sleep 30
        exit 1
    fi

    log "Starting Claude loop (iteration $ITERATION, max $MAX_ITERATIONS)"

    local consecutive_failures=0
    local max_consecutive_failures=5

    while [ $ITERATION -lt $MAX_ITERATIONS ]; do
        ITERATION=$((ITERATION + 1))
        save_iteration

        current_phase=$(cat "$PHASE_FILE" 2>/dev/null || echo "executing")

        # Only loop in executing or cleanup phases
        if [ "$current_phase" != "executing" ] && [ "$current_phase" != "cleanup" ] && [ "$current_phase" != "planning" ]; then
            log "Phase is $current_phase, stopping loop"
            break
        fi

        run_claude
        local result=$?

        if [ $result -eq 2 ]; then
            # Auth error - stop and wait
            log "Stopping due to auth error"
            break
        elif [ $result -ne 0 ]; then
            # Claude exited with error
            consecutive_failures=$((consecutive_failures + 1))
            current_phase=$(cat "$PHASE_FILE" 2>/dev/null || echo "unknown")

            if [ $consecutive_failures -ge $max_consecutive_failures ]; then
                log "ERROR: $consecutive_failures consecutive failures, stopping"
                echo "error" > "$PHASE_FILE"
                echo "Too many consecutive failures ($consecutive_failures)" > "$ERROR_FILE"
                break
            fi

            if [ "$current_phase" = "executing" ]; then
                log "Claude exited during executing phase (failure $consecutive_failures/$max_consecutive_failures), restarting in 5 seconds..."
                sleep 5
            else
                log "Loop complete (phase: $current_phase)"
                break
            fi
        else
            # Success or phase transition
            consecutive_failures=0
            current_phase=$(cat "$PHASE_FILE" 2>/dev/null || echo "unknown")
            if [ "$current_phase" != "executing" ] && [ "$current_phase" != "cleanup" ]; then
                break
            fi
        fi
    done

    if [ $ITERATION -ge $MAX_ITERATIONS ]; then
        log "WARNING: Max iterations ($MAX_ITERATIONS) reached"
        echo "max-iterations-reached" > "$PHASE_FILE"
    fi

    log "Loop finished after $ITERATION iterations (phase: $(cat $PHASE_FILE 2>/dev/null || echo 'unknown'))"
}

# Run main if not sourced
if [ "${BASH_SOURCE[0]}" == "${0}" ]; then
    main "$@"
fi
