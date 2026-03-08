import { NextRequest, NextResponse } from "next/server";

import { proxyMultipartToGateway } from "@/lib/api-client/proxy";
import type { ErrorResponse } from "@/lib/api-client/lumina-gateway";

export async function POST(request: NextRequest) {
  const incoming = await request.formData();
  const file = incoming.get("file");

  if (!(file instanceof File)) {
    const payload: ErrorResponse = {
      error: "missing_file",
      message: "Form field `file` is required.",
      details: null,
    };
    return NextResponse.json(payload, { status: 400 });
  }

  const outgoing = new FormData();
  outgoing.set("file", file, file.name || "upload.pdf");

  return proxyMultipartToGateway("/v1/pdf/extract", outgoing);
}
