import asyncio
import logging
import os

import redis.asyncio as redis
from sllmipy.llms import genai

from ..rqueue import RQueue
from .base_worker import BaseWorker

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)


class GenAIWorker(BaseWorker):
    def __init__(self, redis_client: redis.Redis, queue_name: str):
        self._redis = redis_client
        self._queue = RQueue(self._redis, queue_name)
        self._sentinel = "[DONE]"
        self._worker_id = f"Gen AI worker-{os.getpid()}"
        self._logger = logging.getLogger(self._worker_id)

    async def process(self, task: dict) -> str:
        model_code = task["model_code"]
        prompt = task["prompt"]
        response = await genai.agenerate(model_code, prompt)
        return response

    async def dequeue(self) -> None:
        """Dequeues and processes a single task."""
        self._logger.info("Waiting for a generation task...")

        task = await self._queue.dequeue()
        result_channel = task["task_id"]

        try:
            result = await self.process(task)
            await self._redis.publish(result_channel, result)
        except Exception as e:
            self._logger.error(
                f"Error processing generation task {task['task_id']}: {e}"
            )
            await self._redis.publish(result_channel, f"Error: {e}")
        finally:
            await self._redis.publish(result_channel, self._sentinel)

    async def run(self) -> None:
        """The main loop for the worker."""
        self._logger.info("Worker started.")
        while True:
            await self.dequeue()


async def main():
    redis_client = redis.Redis(
        host="localhost", port=6666, db=0, password="root", decode_responses=True
    )
    worker = GenAIWorker(redis_client, "request_queue")
    await worker.run()


def run():
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        logging.getLogger("worker").info("Worker shutting down.")


if __name__ == "__main__":
    run()
