#!/usr/bin/env bash
# Simple shell-based toy benchmark for testing
# Outputs a score metric based on a configurable value

VALUE=${VALUE:-50}

# Simulate work
sleep 1

# Score: lower is better, optimum at VALUE=42
DIFF=$((VALUE - 42))
if [ $DIFF -lt 0 ]; then
  DIFF=$((-DIFF))
fi

# Simple integer-based score calculation
echo "---"
echo "score:            0.${DIFF}00000"
echo "training_seconds: 1.0"
echo "value:            $VALUE"
