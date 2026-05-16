import asyncio
import json
import logging
import os
import random
import time
from urllib.parse import urlparse

logger = logging.getLogger(__name__)


class RateLimiter:
    def __init__(
        self,
        state_path: str | None = None,
        min_seconds: int = 30,
        max_seconds: int = 90,
    ):
        if state_path is None:
            data_dir = os.environ.get(
                "PIPELINE_DATA_DIR",
                os.path.join(os.path.dirname(__file__), "..", "data", "pipeline"),
            )
            state_path = os.path.join(data_dir, "rate_limits.json")

        self.state_path = state_path
        self.min_seconds = min_seconds
        self.max_seconds = max_seconds
        self._state: dict[str, str] = {}
        self._load()

    def _load(self) -> None:
        try:
            if os.path.exists(self.state_path):
                with open(self.state_path, "r", encoding="utf-8") as f:
                    self._state = json.load(f)
                logger.info("Loaded rate limit state from %s", self.state_path)
        except (json.JSONDecodeError, OSError) as e:
            logger.warning("Failed to load rate limit state: %s", e)
            self._state = {}

    def _save(self) -> None:
        os.makedirs(os.path.dirname(self.state_path), exist_ok=True)
        with open(self.state_path, "w", encoding="utf-8") as f:
            json.dump(self._state, f, indent=2)

    @staticmethod
    def extract_domain(url: str) -> str:
        parsed = urlparse(url)
        return parsed.hostname or url

    async def acquire(self, domain: str) -> float:
        last_access_str = self._state.get(domain)
        delay = random.uniform(self.min_seconds, self.max_seconds)

        if last_access_str:
            try:
                last_access = time.mktime(time.strptime(last_access_str, "%Y-%m-%dT%H:%M:%SZ"))
                elapsed = time.time() - last_access
                wait_time = max(0, delay - elapsed)
            except (ValueError, OSError):
                wait_time = delay
        else:
            wait_time = delay

        if wait_time > 0:
            logger.info(
                "Rate limiting %s: waiting %.1f seconds (delay=%.1f)",
                domain,
                wait_time,
                delay,
            )
            await asyncio.sleep(wait_time)

        now_str = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        self._state[domain] = now_str
        self._save()

        return wait_time