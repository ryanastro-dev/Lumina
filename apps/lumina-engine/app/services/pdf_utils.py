from io import BytesIO

from pypdf import PdfReader


def extract_text_from_pdf_bytes(pdf_bytes: bytes) -> tuple[str, int]:
    reader = PdfReader(BytesIO(pdf_bytes))
    pages: list[str] = []

    for page in reader.pages:
        content = page.extract_text() or ""
        content = content.strip()
        if content:
            pages.append(content)

    return "\n\n".join(pages), len(reader.pages)

