import asyncio
import logging
import os

import redis.asyncio as redis
from pydantic import ValidationError
from sllmipy.llms import genai

from ..config import settings
from ..models import GenerationTask
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

    async def process(self, task: GenerationTask) -> str:
        response = await genai.agenerate(
            task.model_code, task.prompt, config=task.config
        )
        return response

    async def process_stream(self, task: GenerationTask) -> None:
        result_channel = task.task_id
        async for chunk in genai.agenerate_stream(
            task.model_code, task.prompt, config=task.config
        ):
            await self._redis.publish(result_channel, chunk)

    async def dequeue(self) -> None:
        """Dequeues and processes a single task."""
        self._logger.info("Waiting for a generation task...")

        try:
            task_dict = await self._queue.dequeue()
        except redis.exceptions.RedisError as e:
            self._logger.error(f"Failed to dequeue from Redis: {e}. Retrying in 5 seconds.")
            await asyncio.sleep(5)
            return
        except Exception as e:
            self._logger.error(f"Failed to decode task from queue: {e}. Dropping task.")
            return

        if not task_dict:
            return

        try:
            task = GenerationTask.model_validate(task_dict)
        except ValidationError as e:
            self._logger.error(
                f"Task validation failed: {e}. Dropping task: {task_dict}"
            )
            return

        result_channel = task.task_id

        try:
            if task.stream:
                await self.process_stream(task)
            else:
                result = await self.process(task)
                await self._redis.publish(result_channel, result)
        except Exception as e:
            self._logger.error(f"Error processing generation task {task.task_id}: {e}")
            try:
                await self._redis.publish(result_channel, f"Error: {e}")
            except redis.exceptions.RedisError as pub_e:
                self._logger.error(f"Failed to publish error for task {task.task_id}: {pub_e}")
        finally:
            try:
                await self._redis.publish(result_channel, self._sentinel)
            except redis.exceptions.RedisError as pub_e:
                self._logger.error(f"Failed to publish sentinel for task {task.task_id}: {pub_e}")

    async def run(self) -> None:
        """The main loop for the worker."""
        self._logger.info("Worker started.")
        while True:
            await self.dequeue()


async def main():
    redis_client = redis.Redis(
        host=settings.REDIS_HOST,
        port=settings.REDIS_PORT,
        db=settings.REDIS_DB,
        password=settings.REDIS_PASSWORD,
        decode_responses=True,
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
