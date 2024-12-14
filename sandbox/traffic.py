#!/usr/bin/env python3

import argparse
from concurrent.futures import ThreadPoolExecutor, as_completed
import logging
import random
import time
import requests

# Configuration
url = "http://localhost:7777/api/v1/query?query=up"
headers = {}

min_delay = 0.01
max_delay = 1.0


def send_request(i):
    try:
        jitter = random.uniform(min_delay, max_delay)
        time.sleep(jitter)
        response = requests.get(url, headers=headers)
        return f"Request {i}: Status Code {response.status_code}"
    except Exception as e:
        return f"Request {i} failed: {e}"


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Generate traffic for backpressure to block"
    )
    parser.add_argument(
        "-v",
        "--verbose",
        help="Use verbose debug logger (default: False)",
        action="store_true",
        default=False,
    )
    parser.add_argument(
        "-n",
        "--num-requests",
        help="Total requests to send (default: 10)",
        type=int,
        default=10,
    )
    parser.add_argument(
        "-c",
        "--concurrent-workers",
        help="Concurrent workers to process requests (default: 1)",
        type=int,
        default=1,
    )
    return parser.parse_args()


def main() -> None:
    args = parse_arguments()
    level = logging.DEBUG if args.verbose else logging.INFO
    logging.basicConfig(
        format="%(asctime)s - %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
        level=level,
    )
    logger = logging.getLogger(__name__)
    with ThreadPoolExecutor(max_workers=args.concurrent_workers) as executor:
        futures = [executor.submit(send_request, i) for i in range(args.num_requests)]
        for future in as_completed(futures):
            logger.info(future.result())


if __name__ == "__main__":
    main()
