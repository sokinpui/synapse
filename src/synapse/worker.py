import asyncio
import hashlib
import logging
import os
import random

import redis.asyncio as redis

from .rqueue import RQueue

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)


class Worker:
    def __init__(self, redis_client: redis.Redis, queue_name: str):
        self._redis = redis_client
        self._queue = RQueue(self._redis, queue_name)
        self._sentinel = "[DONE]"
        self._worker_id = f"worker-{os.getpid()}"
        self._logger = logging.getLogger(self._worker_id)

    async def _execute_task(self, task: dict) -> str:
        """Simulates the actual work by hashing the prompt."""
        task_length = random.randint(1, 5)
        self._logger.info(
            f"Processing task {task['task_id']}, will take {task_length}s"
        )
        await asyncio.sleep(task_length)
        result = hashlib.sha256(task["prompt"].encode()).hexdigest()
        self._logger.info(f"Finished processing task: {task['task_id']}")
        return result

    async def _process_one(self) -> None:
        """Dequeues and processes a single task."""
        self._logger.info("Waiting for a task...")
        task = await self._queue.dequeue()
        result_channel = task["task_id"]
        try:
            result = await self._execute_task(task)
            await self._redis.publish(result_channel, result)
        except Exception as e:
            self._logger.error(f"Error processing task {task['task_id']}: {e}")
            await self._redis.publish(result_channel, f"Error: {e}")
        finally:
            await self._redis.publish(result_channel, self._sentinel)

    async def run(self) -> None:
        """The main loop for the worker."""
        self._logger.info("Worker started.")
        while True:
            await self._process_one()


async def main():
    redis_client = redis.Redis(
        host="localhost", port=6666, db=0, password="root", decode_responses=True
    )
    worker = Worker(redis_client, "request_queue")
    await worker.run()


def run():
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        logging.getLogger("worker").info("Worker shutting down.")


if __name__ == "__main__":
    run()
