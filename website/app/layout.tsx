import { Provider } from '@/components/provider';
import type { Metadata } from 'next';
import './global.css';

export const metadata: Metadata = {
  title: {
    default: 'gmc',
    template: '%s | gmc',
  },
  description: 'Parallel worktrees for parallel AI agents.',
  metadataBase: new URL(process.env.NEXT_PUBLIC_SITE_URL ?? 'https://gmc.pages.dev'),
};

export default function Layout({ children }: LayoutProps<'/'>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="flex flex-col min-h-screen">
        <Provider>{children}</Provider>
      </body>
    </html>
  );
}
