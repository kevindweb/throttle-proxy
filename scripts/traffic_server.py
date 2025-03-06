#!/usr/bin/env python3

from __future__ import annotations

import os
import random
import time
from typing import Any

from flask import Flask, jsonify

app = Flask(__name__)

# Configuration parameters
LATENCY = float(os.getenv("LATENCY", "0.5"))  # Mock latency in seconds (default 0.5s)
ERROR_RATE = float(os.getenv("ERROR_RATE", "0.1"))  # Rate of 5xx errors (default 10%)
PORT = float(os.getenv("PORT", "6999"))  # Rate of 5xx errors (default 10%)


# Helper function to simulate latency
def simulate_latency() -> None:
    time.sleep(LATENCY)


# Helper function to simulate 5xx errors
def simulate_error() -> bool:
    return random.random() < ERROR_RATE  # noqa: S311 # not crypto


@app.route("/api/data", methods=["GET"])
def get_data() -> tuple[Any, int]:
    simulate_latency()

    if simulate_error():
        return jsonify({"error": "Internal Server Error"}), 500

    return jsonify({"message": "Data successfully retrieved!"}), 200


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=PORT)  # noqa: S104 # allow any fake traffic
