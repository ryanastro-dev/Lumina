from __future__ import annotations

import argparse
import json
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from urllib import error, request

from qdrant_client import QdrantClient
from qdrant_client.http.models import Distance, VectorParams

ROOT_DIR = Path(__file__).resolve().parents[1]
if str(ROOT_DIR) not in sys.path:
    sys.path.insert(0, str(ROOT_DIR))

from app.config import get_settings


@dataclass
class SourceItem:
    id: str
    path: str


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Seed baseline source documents into lumina-engine via /ingest.",
    )
    parser.add_argument(
        "--manifest",
        type=Path,
        default=ROOT_DIR / "data" / "baseline" / "manifest.json",
        help="Path to baseline manifest JSON.",
    )
    parser.add_argument(
        "--engine-base-url",
        type=str,
        default="http://127.0.0.1:8000",
        help="lumina-engine base URL.",
    )
    parser.add_argument(
        "--timeout-seconds",
        type=float,
        default=300.0,
        help="HTTP timeout per ingest request.",
    )
    parser.add_argument(
        "--max-sources",
        type=int,
        default=0,
        help="Optional limit for number of sources to ingest (0 = all).",
    )
    parser.add_argument(
        "--reset-collection",
        action="store_true",
        help="Delete and recreate target Qdrant collection before ingesting.",
    )
    parser.add_argument(
        "--qdrant-url",
        type=str,
        default=None,
        help="Override Qdrant URL used for --reset-collection.",
    )
    parser.add_argument(
        "--qdrant-api-key",
        type=str,
        default=None,
        help="Optional Qdrant API key for --reset-collection.",
    )
    parser.add_argument(
        "--collection",
        type=str,
        default=None,
        help="Override Qdrant collection name for --reset-collection.",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print planned ingest items without calling API.",
    )
    return parser.parse_args()


def load_sources(manifest_path: Path) -> list[SourceItem]:
    data = json.loads(manifest_path.read_text(encoding="utf-8"))
    sources = [SourceItem(**item) for item in data.get("sources", [])]
    if not sources:
        raise ValueError("Manifest has no sources.")
    return sources


def reset_collection_if_needed(args: argparse.Namespace) -> None:
    if not args.reset_collection:
        return

    settings = get_settings()
    collection = args.collection or settings.qdrant_collection
    qdrant_url = args.qdrant_url or settings.qdrant_url
    qdrant_api_key = args.qdrant_api_key or settings.qdrant_api_key

    client = QdrantClient(url=qdrant_url, api_key=qdrant_api_key, timeout=30.0)
    existing = {item.name for item in client.get_collections().collections}
    if collection in existing:
        client.delete_collection(collection_name=collection)

    client.create_collection(
        collection_name=collection,
        vectors_config=VectorParams(size=settings.vector_size, distance=Distance.COSINE),
    )
    print(f"Reset Qdrant collection: {collection} ({qdrant_url})")


def post_ingest(
    engine_base_url: str,
    timeout_seconds: float,
    payload: dict[str, Any],
) -> dict[str, Any]:
    base_url = engine_base_url.rstrip("/")
    url = f"{base_url}/ingest"

    body = json.dumps(payload).encode("utf-8")
    req = request.Request(
        url=url,
        data=body,
        method="POST",
        headers={"Content-Type": "application/json"},
    )

    try:
        with request.urlopen(req, timeout=timeout_seconds) as response:
            data = response.read().decode("utf-8")
            return json.loads(data) if data else {}
    except error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"HTTP {exc.code} from {url}: {detail}") from exc
    except error.URLError as exc:
        raise RuntimeError(f"Unable to reach {url}: {exc.reason}") from exc


def main() -> None:
    args = parse_args()

    manifest_path = args.manifest.resolve()
    root_dir = manifest_path.parent
    sources = load_sources(manifest_path)

    if args.max_sources > 0:
        sources = sources[: args.max_sources]

    if args.reset_collection and args.dry_run:
        print("[dry-run] skipping collection reset")
    else:
        reset_collection_if_needed(args)

    print(f"Manifest: {manifest_path}")
    print(f"Engine: {args.engine_base_url}")
    print(f"Sources to ingest: {len(sources)}")

    for source in sources:
        source_path = root_dir / source.path
        text = source_path.read_text(encoding="utf-8").strip()
        payload = {
            "document_id": source.id,
            "source": source.id,
            "text": text,
            "metadata": {
                "seed": "baseline",
                "path": source.path,
            },
        }

        if args.dry_run:
            print(f"[dry-run] {source.id} <- {source.path} (chars={len(text)})")
            continue

        result = post_ingest(
            engine_base_url=args.engine_base_url,
            timeout_seconds=args.timeout_seconds,
            payload=payload,
        )
        chunk_count = result.get("chunk_count", "?")
        collection = result.get("collection", "?")
        print(f"ingested {source.id} (chunks={chunk_count}, collection={collection})")


if __name__ == "__main__":
    main()

