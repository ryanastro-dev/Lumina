import pytest

from app.services.chunking import chunk_text, normalize_text


def test_normalize_text_collapses_whitespace() -> None:
    text = "Lumina   checks\n\nsemantic\t\tmatches."
    assert normalize_text(text) == "Lumina checks semantic matches."


def test_chunk_text_with_overlap() -> None:
    text = "one two three four five six seven"

    chunks = chunk_text(text, chunk_size=4, overlap=2)

    assert chunks == [
        "one two three four",
        "three four five six",
        "five six seven",
    ]


def test_chunk_text_returns_empty_for_blank_input() -> None:
    assert chunk_text("   \n\t  ", chunk_size=5, overlap=1) == []


@pytest.mark.parametrize(
    ("chunk_size", "overlap"),
    [
        (0, 0),
        (5, -1),
        (5, 5),
    ],
)
def test_chunk_text_validates_window_config(chunk_size: int, overlap: int) -> None:
    with pytest.raises(ValueError):
        chunk_text("one two three", chunk_size=chunk_size, overlap=overlap)
