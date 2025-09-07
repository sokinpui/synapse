import asyncio
import logging
import uuid

import grpc
import redis
import redis.asyncio as aredis

from .config import settings
from .grpc import generate_pb2, generate_pb2_grpc
from .models import GenerationTask
from .rqueue import RQueue

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


class Server(generate_pb2_grpc.GenerateServicer):

    def __init__(self, redis_client: aredis.Redis):
        self._redis = redis_client
        self._q = RQueue(self._redis, "request_queue")
        self._sentinel = "[DONE]"

    async def GenerateTask(self, request, context):
        task_id = str(uuid.uuid4())
        logger.info(f"-> Received request, assigned task_id: {task_id}")
        result_channel = task_id

        pubsub = self._redis.pubsub()
        try:
            await pubsub.subscribe(result_channel)

            config_dict = None
            if request.HasField("config"):
                config_dict = {}
                # We only add fields that are actually set in the request.
                if request.config.HasField("temperature"):
                    config_dict["temperature"] = request.config.temperature
                if request.config.HasField("top_p"):
                    config_dict["top_p"] = request.config.top_p
                if request.config.HasField("top_k"):
                    config_dict["top_k"] = request.config.top_k
                if request.config.HasField("output_length"):
                    config_dict["output_length"] = request.config.output_length

            task = GenerationTask(
                task_id=task_id,
                prompt=request.prompt,
                model_code=request.model_code,
                stream=request.stream,
                config=config_dict,
            )
            await self._q.enqueue(task.model_dump(exclude_none=True))

            output_parts = []
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

            if not request.stream:
                yield generate_pb2.Response(output_string="".join(output_parts))

        except redis.exceptions.ConnectionError as e:
            logger.error(f"A Redis error occurred during task {task_id}: {e}")
            await context.abort(grpc.StatusCode.INTERNAL, "A backend error occurred with Redis.")
        except Exception as e:
            logger.error(f"An unexpected error occurred during task {task_id}: {e}")
            await context.abort(grpc.StatusCode.INTERNAL, "An unexpected server error occurred.")
        finally:
            await pubsub.unsubscribe(result_channel)
            await pubsub.close()


async def serve_async():
    redis_client = aredis.Redis(
        host=settings.REDIS_HOST,
        port=settings.REDIS_PORT,
        db=settings.REDIS_DB,
        password=settings.REDIS_PASSWORD,
        decode_responses=True,
    )
    server = grpc.aio.server()
    generate_pb2_grpc.add_GenerateServicer_to_server(Server(redis_client), server)
    server.add_insecure_port(f"[::]:{settings.GRPC_PORT}")
    await server.start()
    logger.info(f"Server started on port {settings.GRPC_PORT}.")
    await server.wait_for_termination()


def serve():
    try:
        asyncio.run(serve_async())
    except KeyboardInterrupt:
        logger.info("Server shutting down.")


if __name__ == "__main__":
    serve()
