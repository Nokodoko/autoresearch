#!/usr/bin/env python3
"""
Toy benchmark for testing autoresearch.
Simulates a training run that outputs a metric.
The metric is based on a simple function of a MAGIC_NUMBER variable in this file.
"""
import time
import math

# The variable the LLM should optimize
MAGIC_NUMBER = 50

# Simulate some work
time.sleep(2)

# The metric is minimized when MAGIC_NUMBER is close to 42
score = abs(MAGIC_NUMBER - 42) * 0.01 + 0.5

print("---")
print(f"score:            {score:.6f}")
print(f"training_seconds: 2.0")
print(f"magic_number:     {MAGIC_NUMBER}")
