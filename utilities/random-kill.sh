#!/bin/bash

# This utility is used to simulate an unreliable K8s controller (as part of our overall chaos engineering test strategy): 
# For example, this utility can be used to simulate a controller that is constantly running out of memory and being OOMKilled by K8s.
# Even when running under these conditions, our controllers should still work as expected.
# 
# Notes:
# - This utility should only be used in a test/dev environment. NEVER in staging/production.
# - If you see any references to this utility in a staging/prod context, someone messed up :).
# - PRs should never reference utility unless explicitly touching this functionality.

SIGNAL=KILL

# parameters: maximum time between kills (excluding initial time), initial kill-free startup time internal
wait_for_timeout () {
    SLEEP_VAL=$[ ( $RANDOM % $1 ) + $2 ]
    echo "Next kill command in $SLEEP_VAL"
    sleep $SLEEP_VAL
}

# Loop until the user manualy CTRL-C
while true; do

    # (Re)-launch the provided command
    $* &

    # Get the PID
    LAST_PID=$!
    echo "New PID is $LAST_PID" 

    # Wait a random interval
    wait_for_timeout $KILL_INTERVAL $KILL_INITIAL

    # Kill the process
    echo "Killing process: $LAST_PID"
    kill -$SIGNAL $LAST_PID

done
