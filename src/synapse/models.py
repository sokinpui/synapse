from typing import Optional

from pydantic import BaseModel
from sllmipy.config_model import ConfigModel


class GenerationTask(BaseModel):
    task_id: str
    prompt: str
    model_code: str
    stream: bool = False
    config: Optional[ConfigModel] = None
