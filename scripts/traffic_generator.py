#!/usr/bin/env python3

from __future__ import annotations

import argparse
import logging
import random
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field

import requests


@dataclass
class TrafficGeneratorConfig:
    """Configuration for the traffic generator."""

    url: str = "http://localhost:7777/api/v1/query?query=up"
    headers: dict[str, str] = field(default_factory=dict)
    min_delay: float = 0.01
    max_delay: float = 1.0
    num_requests: int = 10
    concurrent_workers: int = 1
    verbose: bool = False


class TrafficGenerator:
    """Manages generation of HTTP traffic with configurable parameters."""

    def __init__(self, config: TrafficGeneratorConfig) -> None:
        """Initialize the traffic generator with given configuration.

        Args:
        ----
            config (TrafficGeneratorConfig): Configuration parameters

        """
        self.config = config
        self.logger = self._setup_logger()

    def _setup_logger(self) -> logging.Logger:
        """Configure and return a logger based on verbosity setting.

        Returns
        -------
            logging.Logger: Configured logger instance

        """
        level = logging.DEBUG if self.config.verbose else logging.INFO
        logging.basicConfig(
            format="%(asctime)s - %(message)s",
            datefmt="%Y-%m-%d %H:%M:%S",
            level=level,
        )
        return logging.getLogger(__name__)

    def _send_request(self, request_id: int) -> str:
        """Send a single HTTP request with jitter.

        Args:
        ----
            request_id (int): Identifier for the request

        Returns:
        -------
            str: Result of the request (status or error)

        """
        try:
            # Add random jitter to distribute request timing
            jitter = random.uniform(self.config.min_delay, self.config.max_delay)  # noqa: S311
            time.sleep(jitter)

            response = requests.get(self.config.url, headers=self.config.headers, timeout=200)
        except Exception as e:
            return f"Request {request_id} failed: {e}"
        else:
            return f"Request {request_id}: Status Code {response.status_code}"

    def generate_traffic(self) -> None:
        """Generate HTTP traffic using thread pool executor. Logs results of each request."""
        with ThreadPoolExecutor(max_workers=self.config.concurrent_workers) as executor:
            futures = [
                executor.submit(self._send_request, i) for i in range(self.config.num_requests)
            ]

            for future in as_completed(futures):
                self.logger.info(future.result())


def parse_arguments() -> TrafficGeneratorConfig:
    """Parse command-line arguments and return a configuration object.

    Returns
    -------
        TrafficGeneratorConfig: Parsed configuration

    """
    parser = argparse.ArgumentParser(description="Generate traffic for backpressure testing")
    parser.add_argument(
        "-v",
        "--verbose",
        help="Use verbose debug logger",
        action="store_true",
        default=False,
    )
    parser.add_argument("-n", "--num-requests", help="Total requests to send", type=int, default=10)
    parser.add_argument(
        "-c",
        "--concurrent-workers",
        help="Concurrent workers to process requests",
        type=int,
        default=1,
    )
    parser.add_argument(
        "--url",
        help="Target URL for requests",
        type=str,
        default="http://localhost:7777/api/v1/query?query=up",
    )

    args = parser.parse_args()
    return TrafficGeneratorConfig(
        url=args.url,
        num_requests=args.num_requests,
        concurrent_workers=args.concurrent_workers,
        verbose=args.verbose,
    )


def main() -> None:
    config = parse_arguments()
    generator = TrafficGenerator(config)
    generator.generate_traffic()


if __name__ == "__main__":
    main()
