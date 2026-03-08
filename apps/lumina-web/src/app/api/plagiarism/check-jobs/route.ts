import { NextRequest } from "next/server";

import { proxyJSONToGateway } from "@/lib/api-client/proxy";

export async function POST(request: NextRequest) {
  return proxyJSONToGateway("/v1/plagiarism/check-jobs", await request.text());
}
