from functools import lru_cache

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )

    app_name: str = "Lumina AI Processing Service"

    qdrant_url: str = "http://localhost:6333"
    qdrant_api_key: str | None = None
    qdrant_collection: str = "lumina_documents"

    embedding_model: str = "sentence-transformers/all-MiniLM-L12-v2"
    vector_size: int = 384

    chunk_size: int = 240
    chunk_overlap: int = 60

    similarity_threshold: float = 0.80
    top_k: int = 5


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    return Settings()
