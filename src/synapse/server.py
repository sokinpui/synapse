import asyncio
import logging
import uuid

import grpc
import redis.asyncio as redis

from .grpc import generate_pb2, generate_pb2_grpc
from .models import GenerationTask
from .rqueue import RQueue

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


class Server(generate_pb2_grpc.GenerateServicer):

    def __init__(self, redis_client: redis.Redis):
        self._redis = redis_client
        self._q = RQueue(self._redis, "request_queue")
        self._sentinel = "[DONE]"

    async def GenerateTask(self, request, context):
        task_id = str(uuid.uuid4())
        logger.info(f"-> Received request, assigned task_id: {task_id}")
        result_channel = task_id

        pubsub = self._redis.pubsub()
        await pubsub.subscribe(result_channel)

        output_parts = []
        try:
            task = GenerationTask(
                task_id=task_id,
                prompt=request.prompt,
                model_code=request.model_code,
                stream=request.stream,
            )
            await self._q.enqueue(task.model_dump())

            async for message in pubsub.listen():
                if message["type"] != "message":
                    continue

                data = message["data"]
                if data == self._sentinel:
                    break

                if request.stream:
                    yield generate_pb2.Response(output_string=data)
                else:
                    output_parts.append(data)
        finally:
            await pubsub.unsubscribe(result_channel)
            await pubsub.close()

        if not request.stream:
            yield generate_pb2.Response(output_string="".join(output_parts))


async def serve_async():
    redis_client = redis.Redis(
        host="localhost", port=6666, db=0, password="root", decode_responses=True
    )
    server = grpc.aio.server()
    generate_pb2_grpc.add_GenerateServicer_to_server(Server(redis_client), server)
    server.add_insecure_port("[::]:50051")
    await server.start()
    logger.info("Server started on port 50051.")
    await server.wait_for_termination()


def serve():
    try:
        asyncio.run(serve_async())
    except KeyboardInterrupt:
        logger.info("Server shutting down.")


if __name__ == "__main__":
    serve()
