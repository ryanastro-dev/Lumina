from app.services.matching import exact_similarity


def test_exact_similarity_identical_text_is_one() -> None:
    text = "Plagiarism detection uses semantic search and exact overlap."
    assert exact_similarity(text, text) == 1.0


def test_exact_similarity_unrelated_text_is_zero() -> None:
    left = "Solar panels convert sunlight into electricity."
    right = "Marine ecosystems depend on coral reef health."
    assert exact_similarity(left, right) == 0.0


def test_exact_similarity_partial_overlap_is_between_zero_and_one() -> None:
    left = "AI systems should cite sources and avoid copied phrasing."
    right = "AI systems should cite sources when reusing phrasing."

    score = exact_similarity(left, right)

    assert 0.0 < score < 1.0
