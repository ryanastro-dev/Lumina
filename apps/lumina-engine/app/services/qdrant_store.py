from uuid import uuid4

from qdrant_client import QdrantClient
from qdrant_client.http.models import Distance, PointStruct, ScoredPoint, VectorParams

from app.config import Settings


class QdrantStore:
    def __init__(self, settings: Settings) -> None:
        self.settings = settings
        self.client = QdrantClient(
            url=settings.qdrant_url,
            api_key=settings.qdrant_api_key,
            timeout=30.0,
        )

    def ensure_collection(self) -> None:
        existing_collections = self.client.get_collections().collections
        existing_names = {collection.name for collection in existing_collections}

        if self.settings.qdrant_collection in existing_names:
            return

        self.client.create_collection(
            collection_name=self.settings.qdrant_collection,
            vectors_config=VectorParams(
                size=self.settings.vector_size,
                distance=Distance.COSINE,
            ),
        )

    def upsert_document(
        self,
        document_id: str,
        chunks: list[str],
        vectors: list[list[float]],
        source: str | None,
        metadata: dict[str, str],
    ) -> int:
        points: list[PointStruct] = []

        for chunk_text, vector in zip(chunks, vectors, strict=True):
            chunk_id = str(uuid4())
            payload = {
                "document_id": document_id,
                "chunk_id": chunk_id,
                "text": chunk_text,
                "source": source,
                "metadata": metadata,
            }
            points.append(
                PointStruct(
                    id=str(uuid4()),
                    vector=vector,
                    payload=payload,
                )
            )

        self.client.upsert(
            collection_name=self.settings.qdrant_collection,
            wait=True,
            points=points,
        )
        return len(points)

    def search(self, query_vector: list[float], limit: int) -> list[ScoredPoint]:
        response = self.client.query_points(
            collection_name=self.settings.qdrant_collection,
            query=query_vector,
            limit=limit,
            with_payload=True,
            with_vectors=False,
        )
        return response.points
