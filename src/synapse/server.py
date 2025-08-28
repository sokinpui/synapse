import uuid
from concurrent import futures

import grpc
import redis

from .grpc import generate_pb2, generate_pb2_grpc
from .rqueue import RQueue

_r = redis.Redis(
    host="localhost", port=6666, db=0, password="root", decode_responses=True
)


class Server(generate_pb2_grpc.GenerateServicer):

    _q = RQueue("request_queue")
    _sentinel = "[DONE]"

    def GenerateTask(self, request, context):
        task_id = str(uuid.uuid4())
        result_channel = task_id

        pubsub = _r.pubsub()
        pubsub.subscribe(result_channel)

        output_parts = []
        try:
            task = {
                "task_id": task_id,
                "prompt": request.prompt,
            }
            self._q.enqueue(task)

            for message in pubsub.listen():
                if message["type"] != "message":
                    continue

                data = message["data"]
                if data == self._sentinel:
                    break

                output_parts.append(data)
        finally:
            pubsub.unsubscribe(result_channel)
            pubsub.close()

        return generate_pb2.Response(output_string="".join(output_parts))


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    generate_pb2_grpc.add_GenerateServicer_to_server(Server(), server)
    server.add_insecure_port("[::]:50051")
    server.start()
    print("Server started on port 50051.")
    server.wait_for_termination()


if __name__ == "__main__":
    serve()
