import type { Metadata } from 'next';
import './globals.css';
import { I18nProvider } from '@/components/providers/i18n-provider';
import { ApiRuntimeConfig } from '@/components/api-runtime-config';

export const metadata: Metadata = {
  title: 'sleepwalker.fm',
  description: 'Real Spotify analytics platform',
};

function resolveServerApiUrl(): string {
  return (
    process.env.API_URL ||
    process.env.NEXT_PUBLIC_API_URL ||
    process.env.NEXT_PUBLIC_API_BASE_URL ||
    ''
  ).replace(/\/+$/, '');
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  const apiUrl = resolveServerApiUrl();

  return (
    <html lang="en">
      <head>
        <ApiRuntimeConfig apiUrl={apiUrl} />
      </head>
      <body className="dark bg-background text-foreground">
        <I18nProvider>{children}</I18nProvider>
      </body>
    </html>
  );
}
