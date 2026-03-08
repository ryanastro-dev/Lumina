import { proxyGETToGateway } from "@/lib/api-client/proxy";

type RouteParams = {
  params: Promise<{ jobId: string }>;
};

export async function GET(_request: Request, { params }: RouteParams) {
  const { jobId } = await params;
  return proxyGETToGateway(`/v1/plagiarism/check-jobs/${encodeURIComponent(jobId)}`);
}
