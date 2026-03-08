from __future__ import annotations

import argparse
import json
import sys
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any
from uuid import uuid4

from qdrant_client import QdrantClient
from qdrant_client.http.models import Distance, PointStruct, VectorParams

ROOT_DIR = Path(__file__).resolve().parents[1]
if str(ROOT_DIR) not in sys.path:
    sys.path.insert(0, str(ROOT_DIR))

from app.config import get_settings
from app.services.chunking import chunk_text
from app.services.embedding import embed_texts
from app.services.matching import exact_similarity


@dataclass
class SourceItem:
    id: str
    path: str


@dataclass
class CaseItem:
    id: str
    path: str
    group: str
    expected_plagiarism: bool
    target_source_id: str | None = None


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Calibrate plagiarism threshold from baseline dataset.",
    )
    parser.add_argument(
        "--manifest",
        type=Path,
        default=ROOT_DIR / "data" / "baseline" / "manifest.json",
        help="Path to baseline manifest JSON.",
    )
    parser.add_argument(
        "--collection",
        type=str,
        default="lumina_baseline_eval",
        help="Qdrant collection used for calibration.",
    )
    parser.add_argument(
        "--qdrant-url",
        type=str,
        default=None,
        help="Override Qdrant URL from .env.",
    )
    parser.add_argument(
        "--qdrant-api-key",
        type=str,
        default=None,
        help="Optional Qdrant API key.",
    )
    parser.add_argument(
        "--model",
        type=str,
        default=None,
        help="Embedding model name.",
    )
    parser.add_argument(
        "--chunk-size",
        type=int,
        default=None,
        help="Chunk size (tokens).",
    )
    parser.add_argument(
        "--chunk-overlap",
        type=int,
        default=None,
        help="Chunk overlap (tokens).",
    )
    parser.add_argument(
        "--top-k",
        type=int,
        default=None,
        help="Top-K retrieval for each query chunk.",
    )
    parser.add_argument(
        "--thresholds",
        type=str,
        default=None,
        help="Comma-separated thresholds, e.g. 0.70,0.75,0.80.",
    )
    parser.add_argument(
        "--threshold-start",
        type=float,
        default=0.60,
        help="Start threshold (used if --thresholds is not provided).",
    )
    parser.add_argument(
        "--threshold-end",
        type=float,
        default=0.95,
        help="End threshold (inclusive if reachable by step).",
    )
    parser.add_argument(
        "--threshold-step",
        type=float,
        default=0.05,
        help="Threshold step size.",
    )
    parser.add_argument(
        "--default-threshold",
        type=float,
        default=0.80,
        help="Reference threshold used for recommendation tie-break.",
    )
    parser.add_argument(
        "--output-json",
        type=Path,
        default=None,
        help="Optional output path for JSON report.",
    )
    parser.add_argument(
        "--keep-collection",
        action="store_true",
        help="Keep calibration collection in Qdrant after run.",
    )
    return parser.parse_args()


def load_manifest(manifest_path: Path) -> tuple[list[SourceItem], list[CaseItem]]:
    data = json.loads(manifest_path.read_text(encoding="utf-8"))

    sources = [SourceItem(**item) for item in data.get("sources", [])]
    cases = [CaseItem(**item) for item in data.get("cases", [])]
    if not sources:
        raise ValueError("Manifest has no sources.")
    if not cases:
        raise ValueError("Manifest has no cases.")
    return sources, cases


def parse_thresholds(args: argparse.Namespace) -> list[float]:
    if args.thresholds:
        values = [float(value.strip()) for value in args.thresholds.split(",") if value.strip()]
    else:
        if args.threshold_step <= 0:
            raise ValueError("--threshold-step must be > 0.")
        values = []
        current = args.threshold_start
        while current <= args.threshold_end + 1e-9:
            values.append(round(current, 4))
            current += args.threshold_step

    normalized = sorted({round(value, 4) for value in values if 0.0 <= value <= 1.0})
    if not normalized:
        raise ValueError("No valid thresholds found in [0.0, 1.0].")
    return normalized


def ensure_clean_collection(client: QdrantClient, collection_name: str, vector_size: int) -> None:
    existing = {collection.name for collection in client.get_collections().collections}
    if collection_name in existing:
        client.delete_collection(collection_name=collection_name)

    client.create_collection(
        collection_name=collection_name,
        vectors_config=VectorParams(size=vector_size, distance=Distance.COSINE),
    )


def index_sources(
    client: QdrantClient,
    collection_name: str,
    root_dir: Path,
    sources: list[SourceItem],
    model_name: str,
    chunk_size: int,
    overlap: int,
) -> int:
    total_chunks = 0

    for source in sources:
        source_path = root_dir / source.path
        text = source_path.read_text(encoding="utf-8").strip()
        chunks = chunk_text(text, chunk_size=chunk_size, overlap=overlap)
        vectors = embed_texts(chunks, model_name=model_name)

        points: list[PointStruct] = []
        for chunk_index, (chunk, vector) in enumerate(zip(chunks, vectors, strict=True)):
            points.append(
                PointStruct(
                    id=str(uuid4()),
                    vector=vector,
                    payload={
                        "source_id": source.id,
                        "source_path": source.path,
                        "chunk_index": chunk_index,
                        "text": chunk,
                    },
                )
            )

        if points:
            client.upsert(collection_name=collection_name, points=points, wait=True)
        total_chunks += len(points)

    return total_chunks


def score_case(
    client: QdrantClient,
    collection_name: str,
    case_text: str,
    model_name: str,
    chunk_size: int,
    overlap: int,
    top_k: int,
) -> dict[str, Any]:
    query_chunks = chunk_text(case_text, chunk_size=chunk_size, overlap=overlap)
    vectors = embed_texts(query_chunks, model_name=model_name)

    best_score = 0.0
    best_semantic = 0.0
    best_exact = 0.0
    best_source_id: str | None = None

    for query_chunk, query_vector in zip(query_chunks, vectors, strict=True):
        response = client.query_points(
            collection_name=collection_name,
            query=query_vector,
            limit=top_k,
            with_payload=True,
            with_vectors=False,
        )
        for point in response.points:
            payload = point.payload or {}
            matched_text = str(payload.get("text", ""))
            semantic = float(point.score or 0.0)
            exact = exact_similarity(query_chunk, matched_text)
            final = max(semantic, exact)

            if final > best_score:
                best_score = final
                best_semantic = semantic
                best_exact = exact
                source_id = payload.get("source_id")
                best_source_id = str(source_id) if source_id is not None else None

    return {
        "score": round(best_score, 6),
        "semantic_score": round(best_semantic, 6),
        "exact_score": round(best_exact, 6),
        "best_source_id": best_source_id,
    }


def compute_metrics(case_results: list[dict[str, Any]], threshold: float) -> dict[str, Any]:
    tp = fp = tn = fn = 0
    for result in case_results:
        expected = bool(result["expected_plagiarism"])
        predicted = float(result["score"]) >= threshold
        if expected and predicted:
            tp += 1
        elif expected and not predicted:
            fn += 1
        elif not expected and predicted:
            fp += 1
        else:
            tn += 1

    precision = tp / (tp + fp) if (tp + fp) else 0.0
    recall = tp / (tp + fn) if (tp + fn) else 0.0
    accuracy = (tp + tn) / (tp + fp + tn + fn) if (tp + fp + tn + fn) else 0.0
    f1 = (2 * precision * recall / (precision + recall)) if (precision + recall) else 0.0

    return {
        "threshold": round(threshold, 4),
        "tp": tp,
        "fp": fp,
        "tn": tn,
        "fn": fn,
        "precision": round(precision, 4),
        "recall": round(recall, 4),
        "f1": round(f1, 4),
        "accuracy": round(accuracy, 4),
    }


def recommend_threshold(
    metrics: list[dict[str, Any]],
    default_threshold: float,
) -> dict[str, Any]:
    return max(
        metrics,
        key=lambda item: (
            item["f1"],
            item["precision"],
            item["recall"],
            -abs(item["threshold"] - default_threshold),
        ),
    )


def main() -> None:
    args = parse_args()
    settings = get_settings()

    model_name = args.model or settings.embedding_model
    chunk_size = args.chunk_size if args.chunk_size is not None else settings.chunk_size
    overlap = args.chunk_overlap if args.chunk_overlap is not None else settings.chunk_overlap
    top_k = args.top_k if args.top_k is not None else settings.top_k
    thresholds = parse_thresholds(args)

    if overlap >= chunk_size:
        raise ValueError("Chunk overlap must be smaller than chunk size.")

    manifest_path = args.manifest.resolve()
    root_dir = manifest_path.parent
    sources, cases = load_manifest(manifest_path)

    qdrant_url = args.qdrant_url or settings.qdrant_url
    qdrant_api_key = args.qdrant_api_key or settings.qdrant_api_key
    client = QdrantClient(url=qdrant_url, api_key=qdrant_api_key, timeout=30.0)

    collection_name = args.collection
    collection_created = False

    try:
        sample_text = [(root_dir / sources[0].path).read_text(encoding="utf-8").strip()]
        sample_vector = embed_texts(sample_text, model_name=model_name)
        if not sample_vector:
            raise ValueError("Unable to infer vector size from sample source.")
        vector_size = len(sample_vector[0])

        ensure_clean_collection(client, collection_name=collection_name, vector_size=vector_size)
        collection_created = True
        total_chunks = index_sources(
            client=client,
            collection_name=collection_name,
            root_dir=root_dir,
            sources=sources,
            model_name=model_name,
            chunk_size=chunk_size,
            overlap=overlap,
        )

        case_results: list[dict[str, Any]] = []
        for case in cases:
            case_path = root_dir / case.path
            case_text = case_path.read_text(encoding="utf-8").strip()
            score = score_case(
                client=client,
                collection_name=collection_name,
                case_text=case_text,
                model_name=model_name,
                chunk_size=chunk_size,
                overlap=overlap,
                top_k=top_k,
            )
            case_results.append(
                {
                    "id": case.id,
                    "group": case.group,
                    "expected_plagiarism": case.expected_plagiarism,
                    "target_source_id": case.target_source_id,
                    **score,
                }
            )

        metrics = [compute_metrics(case_results, threshold=value) for value in thresholds]
        best = recommend_threshold(metrics, default_threshold=args.default_threshold)

        print(f"Manifest: {manifest_path}")
        print(f"Sources indexed: {len(sources)} (chunks={total_chunks})")
        print(f"Cases evaluated: {len(case_results)}")
        print(f"Model: {model_name}")
        print(f"Top-K: {top_k}, Chunk size: {chunk_size}, Overlap: {overlap}")
        print("")
        print("Case scores:")
        for result in case_results:
            print(
                f"- {result['id']:<18} group={result['group']:<16} "
                f"expected={str(result['expected_plagiarism']):<5} "
                f"score={result['score']:.4f} best_source={result['best_source_id']}"
            )

        print("")
        print("Threshold metrics:")
        for metric in metrics:
            print(
                f"- t={metric['threshold']:.2f} "
                f"P={metric['precision']:.3f} "
                f"R={metric['recall']:.3f} "
                f"F1={metric['f1']:.3f} "
                f"Acc={metric['accuracy']:.3f} "
                f"(tp={metric['tp']} fp={metric['fp']} tn={metric['tn']} fn={metric['fn']})"
            )

        print("")
        print(
            "Recommended threshold: "
            f"{best['threshold']:.2f} (F1={best['f1']:.3f}, "
            f"P={best['precision']:.3f}, R={best['recall']:.3f})"
        )

        if args.output_json:
            report = {
                "manifest": str(manifest_path),
                "config": {
                    "collection": collection_name,
                    "qdrant_url": qdrant_url,
                    "model": model_name,
                    "chunk_size": chunk_size,
                    "chunk_overlap": overlap,
                    "top_k": top_k,
                    "thresholds": thresholds,
                },
                "sources": [asdict(source) for source in sources],
                "cases": case_results,
                "metrics": metrics,
                "recommended_threshold": best,
            }
            args.output_json.parent.mkdir(parents=True, exist_ok=True)
            args.output_json.write_text(json.dumps(report, indent=2), encoding="utf-8")
            print(f"Saved report: {args.output_json.resolve()}")

    finally:
        if collection_created and not args.keep_collection:
            client.delete_collection(collection_name=collection_name)


if __name__ == "__main__":
    main()
