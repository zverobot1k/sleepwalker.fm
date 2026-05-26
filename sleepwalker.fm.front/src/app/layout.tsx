import type { Metadata } from 'next';
import './globals.css';
import { I18nProvider } from '@/components/providers/i18n-provider';

export const metadata: Metadata = {
  title: 'sleepwalker.fm',
  description: 'Real Spotify analytics platform',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="dark bg-background text-foreground">
        <I18nProvider>{children}</I18nProvider>
      </body>
    </html>
  );
}
