"use client";

import { FormEvent, useMemo, useRef, useState } from "react";

import {
  GatewayApiError,
  cancelCheckJob,
  extractPdf,
  getCheckJob,
  ingestDocument,
  submitCheckJob,
  type CheckJobStatusResponse,
  type CheckRequest,
  type CheckResponse,
  type IngestResponse,
  type PdfExtractResponse,
} from "@/lib/api-client/lumina-gateway";

type ResultState = {
  ingest: IngestResponse | null;
  check: CheckResponse | null;
  extract: PdfExtractResponse | null;
};

const initialResultState: ResultState = {
  ingest: null,
  check: null,
  extract: null,
};

const checkPollIntervalMs = 1500;
const checkPollTimeoutMs = 120000;

function toErrorMessage(error: unknown): string {
  if (error instanceof GatewayApiError) {
    return `[${error.status}] ${error.code}: ${error.message}`;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return "Unexpected error";
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

export function PlagiarismWorkbench() {
  const [ingestText, setIngestText] = useState("");
  const [source, setSource] = useState("manual-source");
  const [checkText, setCheckText] = useState("");
  const [threshold, setThreshold] = useState("0.8");
  const [topK, setTopK] = useState("5");
  const [selectedFile, setSelectedFile] = useState<File | null>(null);

  const [loadingSection, setLoadingSection] = useState<"ingest" | "check" | "extract" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ResultState>(initialResultState);
  const [checkJob, setCheckJob] = useState<CheckJobStatusResponse | null>(null);
  const [lastCheckRequest, setLastCheckRequest] = useState<CheckRequest | null>(null);

  const currentCheckRunRef = useRef(0);

  const canSubmitCheck = useMemo(() => checkText.trim().length > 0, [checkText]);

  function buildCheckRequestFromInputs(): CheckRequest {
    const parsedThreshold = Number(threshold);
    const parsedTopK = Number(topK);

    return {
      text: checkText,
      threshold: Number.isFinite(parsedThreshold) ? parsedThreshold : undefined,
      top_k: Number.isFinite(parsedTopK) ? parsedTopK : undefined,
    };
  }

  async function executeAsyncCheckFlow(request: CheckRequest): Promise<void> {
    setError(null);
    setLoadingSection("check");
    setResult((prev) => ({ ...prev, check: null }));
    setCheckJob(null);

    const runID = currentCheckRunRef.current + 1;
    currentCheckRunRef.current = runID;

    try {
      const submitResponse = await submitCheckJob(request);
      setCheckJob({
        job_id: submitResponse.job_id,
        status: submitResponse.status,
        created_at: new Date().toISOString(),
      });

      const deadline = Date.now() + checkPollTimeoutMs;
      while (Date.now() < deadline) {
        if (currentCheckRunRef.current !== runID) {
          return;
        }

        const status = await getCheckJob(submitResponse.job_id);
        if (currentCheckRunRef.current !== runID) {
          return;
        }

        setCheckJob(status);

        if (status.status === "completed") {
          if (!status.result) {
            throw new Error("Check job completed without a result payload.");
          }
          setResult((prev) => ({ ...prev, check: status.result ?? null }));
          return;
        }

        if (status.status === "failed" || status.status === "canceled") {
          throw new Error(status.error ?? `Check job ${status.status}.`);
        }

        await sleep(checkPollIntervalMs);
      }

      throw new Error("Check job timed out before completion.");
    } catch (e) {
      if (currentCheckRunRef.current === runID) {
        setError(toErrorMessage(e));
      }
    } finally {
      if (currentCheckRunRef.current === runID) {
        setLoadingSection(null);
      }
    }
  }

  async function cancelCurrentCheck() {
    if (loadingSection !== "check") {
      return;
    }

    const activeJobID = checkJob?.job_id;
    currentCheckRunRef.current += 1;
    setLoadingSection(null);
    setError(null);

    if (activeJobID && activeJobID !== "n/a") {
      try {
        const canceled = await cancelCheckJob(activeJobID);
        setCheckJob((previous) => ({
          ...(previous ?? {
            job_id: canceled.job_id,
            created_at: new Date().toISOString(),
          }),
          status: canceled.status,
          error: "Canceled by user.",
          completed_at: new Date().toISOString(),
        }));
        return;
      } catch (e) {
        setError(toErrorMessage(e));
      }
    }

    setCheckJob((previous) => {
      if (previous) {
        return {
          ...previous,
          status: "canceled",
          error: "Canceled by user.",
          completed_at: new Date().toISOString(),
        };
      }
      return {
        job_id: "n/a",
        status: "canceled",
        created_at: new Date().toISOString(),
        completed_at: new Date().toISOString(),
        error: "Canceled by user.",
      };
    });
  }

  async function onRetryLastCheck() {
    if (!lastCheckRequest || loadingSection !== null) {
      return;
    }

    await executeAsyncCheckFlow(lastCheckRequest);
  }

  async function onIngestSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setLoadingSection("ingest");

    try {
      const ingestResult = await ingestDocument({
        text: ingestText,
        source: source || undefined,
      });
      setResult((prev) => ({ ...prev, ingest: ingestResult }));
    } catch (e) {
      setError(toErrorMessage(e));
    } finally {
      setLoadingSection(null);
    }
  }

  async function onCheckSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const request = buildCheckRequestFromInputs();
    setLastCheckRequest(request);
    await executeAsyncCheckFlow(request);
  }

  async function onExtractSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedFile) {
      setError("Choose a PDF file first.");
      return;
    }

    setError(null);
    setLoadingSection("extract");

    try {
      const extractResult = await extractPdf(selectedFile);
      setResult((prev) => ({ ...prev, extract: extractResult }));
    } catch (e) {
      setError(toErrorMessage(e));
    } finally {
      setLoadingSection(null);
    }
  }

  return (
    <section className="grid">
      <form className="card" onSubmit={onIngestSubmit}>
        <h2>Ingest Source</h2>
        <p>Store source text into vector storage through `lumina-api`.</p>

        <label htmlFor="source">Source</label>
        <input id="source" value={source} onChange={(e) => setSource(e.target.value)} placeholder="source name" />

        <label htmlFor="ingest-text">Text</label>
        <textarea
          id="ingest-text"
          value={ingestText}
          onChange={(e) => setIngestText(e.target.value)}
          placeholder="Paste source text for ingestion"
          required
        />

        <button type="submit" disabled={loadingSection !== null}>
          {loadingSection === "ingest" ? "Ingesting..." : "Ingest"}
        </button>

        {result.ingest ? <pre className="result">{JSON.stringify(result.ingest, null, 2)}</pre> : null}
      </form>

      <form className="card" onSubmit={onCheckSubmit}>
        <h2>Check Similarity</h2>
        <p>Submit async check job, then poll until result is ready.</p>

        <label htmlFor="check-text">Text</label>
        <textarea
          id="check-text"
          value={checkText}
          onChange={(e) => setCheckText(e.target.value)}
          placeholder="Paste text to check"
          required
        />

        <label htmlFor="threshold">Threshold</label>
        <input id="threshold" value={threshold} onChange={(e) => setThreshold(e.target.value)} inputMode="decimal" />

        <label htmlFor="top-k">Top K</label>
        <input id="top-k" value={topK} onChange={(e) => setTopK(e.target.value)} inputMode="numeric" />

        <div className="button-row">
          <button type="submit" disabled={!canSubmitCheck || loadingSection !== null}>
            {loadingSection === "check" ? "Queueing + Polling..." : "Run Check"}
          </button>
          <button
            type="button"
            className="button-secondary"
            onClick={cancelCurrentCheck}
            disabled={loadingSection !== "check"}
          >
            Cancel
          </button>
          <button
            type="button"
            className="button-secondary"
            onClick={onRetryLastCheck}
            disabled={loadingSection !== null || !lastCheckRequest}
          >
            Retry
          </button>
        </div>

        {checkJob ? (
          <div className="result">
            <div>job_id: {checkJob.job_id}</div>
            <div>status: {checkJob.status}</div>
            {checkJob.started_at ? <div>started_at: {checkJob.started_at}</div> : null}
            {checkJob.completed_at ? <div>completed_at: {checkJob.completed_at}</div> : null}
            {checkJob.error ? <div>error: {checkJob.error}</div> : null}
          </div>
        ) : null}

        {result.check ? (
          <div className="result">
            <div>
              is_plagiarism: <strong>{String(result.check.is_plagiarism)}</strong>
            </div>
            <div>overall_score: {result.check.overall_score.toFixed(4)}</div>
            <div>threshold: {result.check.threshold.toFixed(4)}</div>
            <ul className="match-list">
              {result.check.matches.map((match) => (
                <li key={`${match.document_id}-${match.chunk_id}`}>
                  {match.document_id} ({(match.final_score * 100).toFixed(2)}%)
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </form>

      <form className="card" onSubmit={onExtractSubmit}>
        <h2>PDF Extract</h2>
        <p>Upload a PDF and proxy extraction through `lumina-api`.</p>

        <label htmlFor="pdf">PDF file</label>
        <input
          id="pdf"
          type="file"
          accept="application/pdf"
          onChange={(e) => setSelectedFile(e.target.files?.[0] ?? null)}
          required
        />

        <button type="submit" disabled={loadingSection !== null || !selectedFile}>
          {loadingSection === "extract" ? "Extracting..." : "Extract PDF"}
        </button>

        {result.extract ? (
          <pre className="result">{JSON.stringify({ ...result.extract, text: `${result.extract.text.slice(0, 500)}...` }, null, 2)}</pre>
        ) : null}
      </form>

      {error ? <p className="error">{error}</p> : null}
    </section>
  );
}
