from pydantic import BaseModel, Field


class ErrorResponse(BaseModel):
    error: str
    message: str
    details: list[dict] | None = None


class HealthResponse(BaseModel):
    status: str


class IngestRequest(BaseModel):
    text: str = Field(min_length=1)
    document_id: str | None = None
    source: str | None = None
    metadata: dict[str, str] = Field(default_factory=dict)


class IngestResponse(BaseModel):
    document_id: str
    chunk_count: int
    collection: str


class CheckRequest(BaseModel):
    text: str = Field(min_length=1)
    top_k: int | None = Field(default=None, ge=1, le=20)
    threshold: float | None = Field(default=None, ge=0.0, le=1.0)


class MatchResult(BaseModel):
    document_id: str
    source: str | None = None
    chunk_id: str
    matched_text: str
    semantic_score: float
    exact_score: float
    final_score: float


class CheckResponse(BaseModel):
    is_plagiarism: bool
    overall_score: float
    threshold: float
    matches: list[MatchResult]


class PdfExtractResponse(BaseModel):
    filename: str
    page_count: int
    character_count: int
    text: str

