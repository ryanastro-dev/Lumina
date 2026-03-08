from functools import lru_cache

from sentence_transformers import SentenceTransformer


@lru_cache(maxsize=2)
def _load_model(model_name: str) -> SentenceTransformer:
    return SentenceTransformer(model_name)


def embed_texts(texts: list[str], model_name: str) -> list[list[float]]:
    if not texts:
        return []

    model = _load_model(model_name)
    vectors = model.encode(
        texts,
        normalize_embeddings=True,
        show_progress_bar=False,
    )
    return vectors.tolist()
