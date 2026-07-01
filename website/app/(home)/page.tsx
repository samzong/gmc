import Link from 'next/link';
import { LatestRelease } from '@/components/latest-release';

export default function HomePage() {
  return (
    <main className="mx-auto flex w-full max-w-3xl flex-1 flex-col justify-center px-6 py-20">
      <h1 className="mb-3 text-5xl font-semibold tracking-tight">gmc</h1>
      <p className="mb-8 text-lg text-fd-muted-foreground">
        Parallel worktrees for parallel AI agents.
      </p>
      <div className="mb-6 overflow-hidden rounded-lg border bg-fd-card">
        <div className="border-b px-4 py-2 text-sm text-fd-muted-foreground">Install</div>
        <pre className="overflow-x-auto px-4 py-4 text-sm">
          <code>brew install samzong/tap/gmc</code>
        </pre>
      </div>
      <LatestRelease />
      <Link
        href="/docs"
        className="inline-flex h-10 w-fit items-center rounded-md bg-fd-primary px-4 text-sm font-medium text-fd-primary-foreground"
      >
        Read the docs
      </Link>
    </main>
  );
}
