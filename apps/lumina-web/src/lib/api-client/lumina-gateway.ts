import type { components } from "./gateway-apicontract";

export type IngestRequest = components["schemas"]["IngestRequest"];
export type IngestResponse = components["schemas"]["IngestResponse"];
export type CheckRequest = components["schemas"]["CheckRequest"];
export type CheckResponse = components["schemas"]["CheckResponse"];
export type PdfExtractResponse = components["schemas"]["PdfExtractResponse"];
export type ErrorResponse = components["schemas"]["ErrorResponse"];

export type CheckJobAcceptedResponse = components["schemas"]["CheckJobAcceptedResponse"];
export type CheckJobStatusResponse = components["schemas"]["CheckJobStatusResponse"];

export class GatewayApiError extends Error {
  status: number;
  code: string;
  details: ErrorResponse["details"];

  constructor(status: number, payload: ErrorResponse | null, fallbackMessage: string) {
    super(payload?.message ?? fallbackMessage);
    this.name = "GatewayApiError";
    this.status = status;
    this.code = payload?.error ?? "unknown_error";
    this.details = payload?.details;
  }
}

async function parseResponse<T>(response: Response): Promise<T> {
  const text = await response.text();
  let payload: unknown = null;

  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = null;
    }
  }

  if (!response.ok) {
    throw new GatewayApiError(
      response.status,
      payload as ErrorResponse | null,
      `Gateway request failed with status ${response.status}`
    );
  }

  return payload as T;
}

export async function ingestDocument(input: IngestRequest): Promise<IngestResponse> {
  const response = await fetch("/api/plagiarism/ingest", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  return parseResponse<IngestResponse>(response);
}

export async function checkPlagiarism(input: CheckRequest): Promise<CheckResponse> {
  const response = await fetch("/api/plagiarism/check", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  return parseResponse<CheckResponse>(response);
}

export async function submitCheckJob(input: CheckRequest): Promise<CheckJobAcceptedResponse> {
  const response = await fetch("/api/plagiarism/check-jobs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  return parseResponse<CheckJobAcceptedResponse>(response);
}

export async function getCheckJob(jobId: string): Promise<CheckJobStatusResponse> {
  const response = await fetch(`/api/plagiarism/check-jobs/${encodeURIComponent(jobId)}`, {
    method: "GET",
    cache: "no-store",
  });
  return parseResponse<CheckJobStatusResponse>(response);
}

export async function cancelCheckJob(jobId: string): Promise<CheckJobAcceptedResponse> {
  const response = await fetch(`/api/plagiarism/check-jobs/${encodeURIComponent(jobId)}/cancel`, {
    method: "POST",
  });
  return parseResponse<CheckJobAcceptedResponse>(response);
}

export async function extractPdf(file: File): Promise<PdfExtractResponse> {
  const formData = new FormData();
  formData.set("file", file, file.name);

  const response = await fetch("/api/pdf/extract", {
    method: "POST",
    body: formData,
  });
  return parseResponse<PdfExtractResponse>(response);
}

