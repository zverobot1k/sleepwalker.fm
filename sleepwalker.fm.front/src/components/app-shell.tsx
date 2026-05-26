'use client';

import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { BarChart3, Sparkles, ListMusic, Download, Globe, LogOut, Moon } from 'lucide-react';
import { clearSession, getSession } from '@/lib/session';
import { useI18n } from '@/components/providers/i18n-provider';

const items = [
  { href: '/dashboard', key: 'dashboard', icon: BarChart3 },
  { href: '/wrapped', key: 'wrapped', icon: Sparkles },
  { href: '/recommendations', key: 'recommendations', icon: ListMusic },
  { href: '/playlist', key: 'playlist', icon: Download },
];

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { t, lang, setLang } = useI18n();
  const session = getSession();

  return (
    <div className="min-h-screen bg-background text-foreground relative overflow-hidden">
      <div className="fixed inset-0 bg-gradient-to-br from-[#0a0118] via-[#1a0b2e] to-[#0a0118] -z-10" />
      <div className="fixed inset-0 bg-[radial-gradient(ellipse_at_top,_rgba(139,92,246,0.15),transparent_50%)] -z-10" />
      <div className="fixed inset-0 bg-[radial-gradient(ellipse_at_bottom_right,_rgba(99,102,241,0.1),transparent_50%)] -z-10" />

      <div className="flex min-h-screen">
        <aside className="w-72 border-r border-glass-border bg-glass-bg backdrop-blur-xl p-6 flex flex-col">
          <div className="mb-10 flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-violet-500 to-indigo-600 flex items-center justify-center">
              <Moon className="w-5 h-5 text-white" />
            </div>
            <div>
              <p className="text-lg font-semibold">sleepwalker.fm</p>
              <p className="text-xs text-muted-foreground">Spotify Analytics</p>
            </div>
          </div>

          <nav className="space-y-2 flex-1">
            {items.map((item) => {
              const Icon = item.icon;
              const active = pathname === item.href;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={`flex items-center gap-3 px-4 py-3 rounded-xl transition ${
                    active
                      ? 'bg-primary/20 text-primary border border-primary/30'
                      : 'text-muted-foreground hover:bg-secondary hover:text-foreground'
                  }`}
                >
                  <Icon className="w-4 h-4" />
                  <span>{t(item.key)}</span>
                </Link>
              );
            })}
          </nav>

          <div className="space-y-3 border-t border-glass-border pt-4">
            <button
              onClick={() => setLang(lang === 'en' ? 'ru' : 'en')}
              className="w-full px-4 py-2 rounded-lg bg-secondary/60 hover:bg-secondary transition flex items-center justify-center gap-2"
            >
              <Globe className="w-4 h-4" />
              {lang === 'en' ? 'Русский' : 'English'}
            </button>
            <div className="text-xs text-muted-foreground px-2 truncate">
              {session?.displayName || session?.email || session?.userId}
            </div>
            <button
              onClick={() => {
                clearSession();
                router.replace('/');
              }}
              className="w-full px-4 py-2 rounded-lg bg-red-500/20 text-red-300 hover:bg-red-500/30 transition flex items-center justify-center gap-2"
            >
              <LogOut className="w-4 h-4" />
              {t('logout')}
            </button>
          </div>
        </aside>

        <main className="flex-1 p-8">{children}</main>
      </div>
    </div>
  );
}
