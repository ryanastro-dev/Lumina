import re


_TOKEN_PATTERN = re.compile(r"[a-z0-9]+")


def _tokenize(text: str) -> list[str]:
    return _TOKEN_PATTERN.findall(text.lower())


def _shingles(text: str, size: int = 5) -> set[str]:
    tokens = _tokenize(text)
    if not tokens:
        return set()
    if len(tokens) < size:
        return {" ".join(tokens)}
    return {" ".join(tokens[i : i + size]) for i in range(len(tokens) - size + 1)}


def exact_similarity(left: str, right: str) -> float:
    left_set = _shingles(left)
    right_set = _shingles(right)

    if not left_set or not right_set:
        return 0.0

    intersection = left_set.intersection(right_set)
    union = left_set.union(right_set)
    return len(intersection) / len(union)
