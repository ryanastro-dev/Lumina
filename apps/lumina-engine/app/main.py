from contextlib import asynccontextmanager
from uuid import uuid4

from fastapi import FastAPI, File, HTTPException, Request, UploadFile
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from app.config import get_settings
from app.errors import raise_api_error
from app.models import (
    CheckRequest,
    CheckResponse,
    ErrorResponse,
    HealthResponse,
    IngestRequest,
    IngestResponse,
    MatchResult,
    PdfExtractResponse,
)
from app.services.chunking import chunk_text
from app.services.embedding import embed_texts
from app.services.matching import exact_similarity
from app.services.pdf_utils import extract_text_from_pdf_bytes
from app.services.qdrant_store import QdrantStore

settings = get_settings()
store = QdrantStore(settings)


@asynccontextmanager
async def lifespan(_: FastAPI):
    try:
        store.ensure_collection()
    except Exception as exc:  # noqa: BLE001
        raise RuntimeError(
            "Unable to connect to Qdrant. Start Qdrant before running the API."
        ) from exc
    yield


app = FastAPI(
    title=settings.app_name,
    version="1.0.0",
    openapi_version="3.0.3",
    lifespan=lifespan,
)


@app.exception_handler(HTTPException)
@app.exception_handler(StarletteHTTPException)
async def handle_http_exception(_: Request, exc: HTTPException) -> JSONResponse:
    detail = exc.detail
    if isinstance(detail, dict) and "error" in detail and "message" in detail:
        payload = detail
    else:
        payload = {
            "error": "http_error",
            "message": str(detail),
            "details": None,
        }
    return JSONResponse(status_code=exc.status_code, content=payload)


@app.exception_handler(RequestValidationError)
async def handle_validation_exception(_: Request, exc: RequestValidationError) -> JSONResponse:
    payload = {
        "error": "validation_error",
        "message": "Request validation failed.",
        "details": exc.errors(),
    }
    return JSONResponse(status_code=422, content=payload)


@app.get(
    "/health",
    response_model=HealthResponse,
    summary="Health check",
)
def health() -> HealthResponse:
    return HealthResponse(status="ok")


@app.post(
    "/ingest",
    response_model=IngestResponse,
    status_code=200,
    summary="Ingest a text document into vector storage",
    responses={
        400: {"model": ErrorResponse, "description": "Invalid text or chunking input."},
        422: {"model": ErrorResponse, "description": "Request validation failed."},
        503: {"model": ErrorResponse, "description": "Qdrant unavailable."},
    },
)
def ingest_document(request: IngestRequest) -> IngestResponse:
    chunks = chunk_text(
        text=request.text,
        chunk_size=settings.chunk_size,
        overlap=settings.chunk_overlap,
    )
    if not chunks:
        raise_api_error(
            status_code=400,
            error="chunking_error",
            message="No text available after chunking.",
        )

    vectors = embed_texts(chunks, settings.embedding_model)
    document_id = request.document_id or str(uuid4())

    try:
        chunk_count = store.upsert_document(
            document_id=document_id,
            chunks=chunks,
            vectors=vectors,
            source=request.source,
            metadata=request.metadata,
        )
    except Exception as exc:  # noqa: BLE001
        raise_api_error(
            status_code=503,
            error="vector_store_unavailable",
            message="Qdrant upsert failed.",
            details={"reason": str(exc)},
        )

    return IngestResponse(
        document_id=document_id,
        chunk_count=chunk_count,
        collection=settings.qdrant_collection,
    )


@app.post(
    "/check",
    response_model=CheckResponse,
    status_code=200,
    summary="Check text similarity against indexed documents",
    responses={
        400: {"model": ErrorResponse, "description": "Invalid text or chunking input."},
        422: {"model": ErrorResponse, "description": "Request validation failed."},
        503: {"model": ErrorResponse, "description": "Qdrant unavailable."},
    },
)
def check_plagiarism(request: CheckRequest) -> CheckResponse:
    threshold = request.threshold if request.threshold is not None else settings.similarity_threshold
    top_k = request.top_k if request.top_k is not None else settings.top_k

    query_chunks = chunk_text(
        text=request.text,
        chunk_size=settings.chunk_size,
        overlap=settings.chunk_overlap,
    )
    if not query_chunks:
        raise_api_error(
            status_code=400,
            error="chunking_error",
            message="No text available after chunking.",
        )

    query_vectors = embed_texts(query_chunks, settings.embedding_model)

    best_by_chunk: dict[tuple[str, str], MatchResult] = {}
    for query_chunk, query_vector in zip(query_chunks, query_vectors, strict=True):
        try:
            candidates = store.search(query_vector=query_vector, limit=top_k)
        except Exception as exc:  # noqa: BLE001
            raise_api_error(
                status_code=503,
                error="vector_store_unavailable",
                message="Qdrant search failed.",
                details={"reason": str(exc)},
            )

        for candidate in candidates:
            payload = candidate.payload or {}
            matched_text = str(payload.get("text", ""))
            document_id = str(payload.get("document_id", ""))
            chunk_id = str(payload.get("chunk_id", ""))
            source = payload.get("source")
            source = str(source) if source is not None else None

            semantic_score = float(candidate.score or 0.0)
            exact_score = exact_similarity(query_chunk, matched_text)
            final_score = max(semantic_score, exact_score)

            key = (document_id, chunk_id)
            previous = best_by_chunk.get(key)
            if previous is not None and previous.final_score >= final_score:
                continue

            best_by_chunk[key] = MatchResult(
                document_id=document_id,
                source=source,
                chunk_id=chunk_id,
                matched_text=matched_text,
                semantic_score=semantic_score,
                exact_score=exact_score,
                final_score=final_score,
            )

    matches = [match for match in best_by_chunk.values() if match.final_score >= threshold]
    matches.sort(key=lambda item: item.final_score, reverse=True)
    overall_score = matches[0].final_score if matches else 0.0

    return CheckResponse(
        is_plagiarism=overall_score >= threshold,
        overall_score=overall_score,
        threshold=threshold,
        matches=matches[: top_k * 3],
    )


@app.post(
    "/extract-pdf",
    response_model=PdfExtractResponse,
    status_code=200,
    summary="Extract plain text from an uploaded PDF",
    responses={
        400: {"model": ErrorResponse, "description": "Invalid PDF input."},
        422: {"model": ErrorResponse, "description": "Request validation failed."},
    },
)
async def extract_pdf(file: UploadFile = File(...)) -> PdfExtractResponse:
    filename = file.filename or ""
    if not filename.lower().endswith(".pdf"):
        raise_api_error(
            status_code=400,
            error="invalid_file_type",
            message="Only PDF files are supported.",
        )

    raw = await file.read()
    if not raw:
        raise_api_error(
            status_code=400,
            error="empty_file",
            message="Uploaded file is empty.",
        )

    try:
        text, page_count = extract_text_from_pdf_bytes(raw)
    except Exception as exc:  # noqa: BLE001
        raise_api_error(
            status_code=400,
            error="pdf_parse_error",
            message="Unable to read PDF.",
            details={"reason": str(exc)},
        )

    return PdfExtractResponse(
        filename=filename,
        page_count=page_count,
        character_count=len(text),
        text=text,
    )


