def normalize_text(text: str) -> str:
    return " ".join(text.split())


def chunk_text(text: str, chunk_size: int, overlap: int) -> list[str]:
    if chunk_size <= 0:
        raise ValueError("chunk_size must be > 0")
    if overlap < 0:
        raise ValueError("overlap must be >= 0")
    if overlap >= chunk_size:
        raise ValueError("overlap must be smaller than chunk_size")

    normalized = normalize_text(text)
    tokens = normalized.split(" ")
    tokens = [token for token in tokens if token]

    if not tokens:
        return []

    step = chunk_size - overlap
    chunks: list[str] = []

    for start in range(0, len(tokens), step):
        window = tokens[start : start + chunk_size]
        if not window:
            break
        chunks.append(" ".join(window))
        if start + chunk_size >= len(tokens):
            break

    return chunks
