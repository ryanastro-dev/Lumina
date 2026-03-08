from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

SERVICE_ROOT = Path(__file__).resolve().parents[1]
REPO_ROOT = Path(__file__).resolve().parents[3]

if str(SERVICE_ROOT) not in sys.path:
    sys.path.insert(0, str(SERVICE_ROOT))

from app.main import app


def _downgrade_31_nullable(node: Any) -> Any:
    if isinstance(node, list):
        return [_downgrade_31_nullable(item) for item in node]

    if not isinstance(node, dict):
        return node

    transformed = {
        key: _downgrade_31_nullable(value)
        for key, value in node.items()
        if key != "$schema"
    }

    any_of = transformed.get("anyOf")
    if isinstance(any_of, list) and len(any_of) >= 2:
        non_null = [
            item
            for item in any_of
            if not (isinstance(item, dict) and item.get("type") == "null")
        ]
        null_count = len(any_of) - len(non_null)

        if null_count == 1 and len(non_null) == 1:
            merged = dict(non_null[0])
            for key, value in transformed.items():
                if key == "anyOf":
                    continue
                merged[key] = value
            merged["nullable"] = True
            return merged

    return transformed


def _to_openapi_30(spec_31: dict[str, Any]) -> dict[str, Any]:
    spec_30 = _downgrade_31_nullable(spec_31)
    spec_30["openapi"] = "3.0.3"
    return spec_30


def main() -> None:
    spec_31 = app.openapi()
    spec_30 = _to_openapi_30(spec_31)
    spec_text = json.dumps(spec_30, indent=2)

    contract_output_path = REPO_ROOT / "contracts" / "openapi" / "lumina-engine-v1.json"
    contract_output_path.parent.mkdir(parents=True, exist_ok=True)
    contract_output_path.write_text(spec_text, encoding="utf-8")

    docs_output_path = REPO_ROOT / "docs" / "openapi-v1.json"
    docs_output_path.parent.mkdir(parents=True, exist_ok=True)
    docs_output_path.write_text(spec_text, encoding="utf-8")

    print(f"OpenAPI spec exported: {contract_output_path}")
    print(f"OpenAPI doc snapshot: {docs_output_path}")
    print("OpenAPI version: 3.0.3 (codegen-compatible)")


if __name__ == "__main__":
    main()
