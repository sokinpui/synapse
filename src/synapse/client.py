import asyncio
import logging

import grpc

from .grpc import generate_pb2, generate_pb2_grpc

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


async def send_request(stub: generate_pb2_grpc.GenerateStub, request_id: int):
    """Sends a single request and logs the outcome."""
    prompt = f"Tell me a story about request number {request_id}."
    logger.info(f"-> [Request {request_id}] Sending...")

    request = generate_pb2.Request(prompt=prompt)

    try:
        response = await stub.GenerateTask(request)
        logger.info(
            f"<- [Request {request_id}] Received response hash: {response.output_string}"
        )
    except grpc.aio.AioRpcError as e:
        logger.error(
            f"<- [Request {request_id}] RPC failed: {e.code()} - {e.details()}"
        )


async def main():
    """
    Connects to the gRPC server and sends multiple requests concurrently.
    """
    async with grpc.aio.insecure_channel("localhost:50051") as channel:
        stub = generate_pb2_grpc.GenerateStub(channel)

        num_requests = 5
        logger.info(f"Starting {num_requests} concurrent requests...")

        tasks = [
            asyncio.create_task(send_request(stub, i)) for i in range(num_requests)
        ]

        await asyncio.gather(*tasks)
        logger.info("All requests have been processed.")


def run():
    """Entry point for the client script."""
    asyncio.run(main())


if __name__ == "__main__":
    run()
