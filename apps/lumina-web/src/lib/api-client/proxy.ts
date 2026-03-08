import { NextResponse } from "next/server";
import type { ErrorResponse } from "./lumina-gateway";

const defaultGatewayBaseURL = "http://localhost:8080";
const gatewayAPIKeyHeader = "X-API-Key";

function normalizeBaseURL(value: string): string {
  return value.replace(/\/+$/, "");
}

export function getGatewayBaseURL(): string {
  const envValue = process.env.LUMINA_GATEWAY_BASE_URL ?? process.env.NEXT_PUBLIC_GATEWAY_BASE_URL;
  const baseURL = envValue && envValue.trim() !== "" ? envValue : defaultGatewayBaseURL;
  return normalizeBaseURL(baseURL);
}

function buildGatewayHeaders(initial?: HeadersInit): Headers {
  const headers = new Headers(initial);
  const apiKey = process.env.LUMINA_GATEWAY_API_KEY;
  if (apiKey && apiKey.trim() !== "") {
    headers.set(gatewayAPIKeyHeader, apiKey.trim());
  }
  return headers;
}

function gatewayUnavailableResponse(error: unknown): NextResponse {
  const payload: ErrorResponse = {
    error: "gateway_unavailable",
    message: "Unable to reach lumina-api gateway.",
    details: [{ reason: error instanceof Error ? error.message : "unknown" }],
  };

  return NextResponse.json(payload, { status: 502 });
}

function toProxyResponse(response: Response, raw: string): NextResponse {
  return new NextResponse(raw, {
    status: response.status,
    headers: { "Content-Type": response.headers.get("content-type") ?? "application/json" },
  });
}

export async function proxyJSONToGateway(upstreamPath: string, body: string): Promise<NextResponse> {
  try {
    const response = await fetch(`${getGatewayBaseURL()}${upstreamPath}`, {
      method: "POST",
      headers: buildGatewayHeaders({ "Content-Type": "application/json" }),
      body,
      cache: "no-store",
    });

    const raw = await response.text();
    return toProxyResponse(response, raw);
  } catch (error) {
    return gatewayUnavailableResponse(error);
  }
}

export async function proxyPOSTToGateway(upstreamPath: string): Promise<NextResponse> {
  try {
    const response = await fetch(`${getGatewayBaseURL()}${upstreamPath}`, {
      method: "POST",
      headers: buildGatewayHeaders(),
      cache: "no-store",
    });

    const raw = await response.text();
    return toProxyResponse(response, raw);
  } catch (error) {
    return gatewayUnavailableResponse(error);
  }
}

export async function proxyGETToGateway(upstreamPath: string): Promise<NextResponse> {
  try {
    const response = await fetch(`${getGatewayBaseURL()}${upstreamPath}`, {
      method: "GET",
      headers: buildGatewayHeaders(),
      cache: "no-store",
    });

    const raw = await response.text();
    return toProxyResponse(response, raw);
  } catch (error) {
    return gatewayUnavailableResponse(error);
  }
}

export async function proxyMultipartToGateway(upstreamPath: string, formData: FormData): Promise<NextResponse> {
  try {
    const response = await fetch(`${getGatewayBaseURL()}${upstreamPath}`, {
      method: "POST",
      headers: buildGatewayHeaders(),
      body: formData,
      cache: "no-store",
    });

    const raw = await response.text();
    return toProxyResponse(response, raw);
  } catch (error) {
    return gatewayUnavailableResponse(error);
  }
}
