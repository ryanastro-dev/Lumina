from typing import NoReturn

from fastapi import HTTPException


def raise_api_error(
    status_code: int,
    error: str,
    message: str,
    details: list[dict] | dict | None = None,
) -> NoReturn:
    normalized_details: list[dict] | None
    if details is None:
        normalized_details = None
    elif isinstance(details, dict):
        normalized_details = [details]
    else:
        normalized_details = details

    raise HTTPException(
        status_code=status_code,
        detail={
            "error": error,
            "message": message,
            "details": normalized_details,
        },
    )
