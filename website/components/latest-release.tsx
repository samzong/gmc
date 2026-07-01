'use client';

import Link from 'next/link';
import { Tag } from 'lucide-react';
import { useEffect, useState } from 'react';

const apiUrl = 'https://api.github.com/repos/samzong/gmc/releases/latest';
const releasesUrl = 'https://github.com/samzong/gmc/releases';

interface Release {
  tag: string;
  url: string;
}

export function LatestRelease() {
  const [release, setRelease] = useState<Release | null>(null);

  useEffect(() => {
    let ignore = false;

    async function load() {
      const response = await fetch(apiUrl, {
        headers: {
          Accept: 'application/vnd.github+json',
        },
      });
      if (!response.ok) return;

      const data = (await response.json()) as { tag_name?: unknown; html_url?: unknown };
      if (ignore || typeof data.tag_name !== 'string' || typeof data.html_url !== 'string') return;

      setRelease({ tag: data.tag_name, url: data.html_url });
    }

    void load().catch(() => {});

    return () => {
      ignore = true;
    };
  }, []);

  return (
    <Link
      href={release?.url ?? releasesUrl}
      className="mb-8 inline-flex w-fit items-center gap-2 rounded-md border bg-fd-card px-3 py-2 text-sm text-fd-muted-foreground transition-colors hover:text-fd-foreground"
      aria-label="Latest gmc release"
    >
      <Tag className="size-4" aria-hidden="true" />
      <span>{release?.tag ?? 'Latest release'}</span>
    </Link>
  );
}
