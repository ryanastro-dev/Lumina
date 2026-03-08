import { PlagiarismWorkbench } from "@/features/plagiarism/plagiarism-workbench";

export default function HomePage() {
  return (
    <main className="page-shell">
      <header className="page-header">
        <p className="eyebrow">Lumina</p>
        <h1>Plagiarism Workbench</h1>
        <p>Use this UI to ingest sources, run checks, and validate PDF extraction through the gateway.</p>
      </header>
      <PlagiarismWorkbench />
    </main>
  );
}
