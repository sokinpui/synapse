from pydantic import BaseModel


class GenerationTask(BaseModel):
    task_id: str
    prompt: str
    model_code: str
    stream: bool = False
