from typing import Any

from app.services import embedding


class _FakeEncodedVectors:
    def __init__(self, values: list[list[float]]) -> None:
        self._values = values

    def tolist(self) -> list[list[float]]:
        return self._values


class _FakeModel:
    def __init__(self) -> None:
        self.calls: list[dict[str, Any]] = []

    def encode(
        self,
        texts: list[str],
        *,
        normalize_embeddings: bool,
        show_progress_bar: bool,
    ) -> _FakeEncodedVectors:
        self.calls.append(
            {
                "texts": texts,
                "normalize_embeddings": normalize_embeddings,
                "show_progress_bar": show_progress_bar,
            }
        )
        return _FakeEncodedVectors([[0.1, 0.2], [0.3, 0.4]])


def test_embed_texts_returns_empty_without_model_load(monkeypatch) -> None:
    def _fail_loader(_: str) -> _FakeModel:
        raise AssertionError("model loader should not be called for empty input")

    monkeypatch.setattr(embedding, "_load_model", _fail_loader)

    assert embedding.embed_texts([], "unused-model") == []


def test_embed_texts_encodes_with_normalized_embeddings(monkeypatch) -> None:
    fake_model = _FakeModel()
    monkeypatch.setattr(embedding, "_load_model", lambda _: fake_model)

    vectors = embedding.embed_texts(["alpha", "beta"], "mock-model")

    assert vectors == [[0.1, 0.2], [0.3, 0.4]]
    assert fake_model.calls == [
        {
            "texts": ["alpha", "beta"],
            "normalize_embeddings": True,
            "show_progress_bar": False,
        }
    ]
