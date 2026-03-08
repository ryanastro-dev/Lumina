import { proxyPOSTToGateway } from "@/lib/api-client/proxy";

type RouteParams = {
  params: Promise<{ jobId: string }>;
};

export async function POST(_request: Request, { params }: RouteParams) {
  const { jobId } = await params;
  return proxyPOSTToGateway(`/v1/plagiarism/check-jobs/${encodeURIComponent(jobId)}/cancel`);
}
