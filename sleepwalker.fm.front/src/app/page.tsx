'use client';

import { motion } from 'framer-motion';
import { Sparkles, Music } from 'lucide-react';
import { useI18n } from '@/components/providers/i18n-provider';

const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL || 'http://localhost:8080';

export default function LandingPage() {
  const { t } = useI18n();

  return (
    <div className="min-h-screen bg-background text-foreground relative overflow-hidden">
      <div className="fixed inset-0 bg-gradient-to-br from-[#0a0118] via-[#1a0b2e] to-[#0a0118] -z-10" />
      <div className="fixed inset-0 bg-[radial-gradient(ellipse_at_top,_rgba(139,92,246,0.15),transparent_50%)] -z-10" />

      <div className="container mx-auto px-6 py-8">
        <header className="flex items-center gap-3 mb-20">
          <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-violet-500 to-indigo-600 flex items-center justify-center">
            <Music className="w-5 h-5 text-white" />
          </div>
          <span className="text-xl font-semibold">sleepwalker.fm</span>
        </header>

        <section className="max-w-4xl mx-auto text-center">
          <motion.div initial={{ opacity: 0, y: 24 }} animate={{ opacity: 1, y: 0 }}>
            <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-secondary/50 border border-glass-border mb-8">
              <Sparkles className="w-4 h-4 text-violet-400" />
              <span className="text-sm text-violet-300">Real Spotify analytics, no mock data</span>
            </div>
            <h1 className="text-6xl md:text-7xl font-bold mb-6 leading-tight bg-gradient-to-r from-violet-200 via-purple-200 to-indigo-200 bg-clip-text text-transparent">
              sleepwalker.fm
            </h1>
            <p className="text-xl text-muted-foreground mb-12">
              Production analytics dashboard powered by real Spotify endpoints.
            </p>
            <button
              onClick={() => {
                window.location.href = `${API_BASE}/auth/spotify/login`;
              }}
              className="px-8 py-4 rounded-full bg-gradient-to-r from-violet-600 to-indigo-600 hover:from-violet-500 hover:to-indigo-500 transition-all duration-300 font-semibold shadow-2xl shadow-violet-500/30"
            >
              {t('connectSpotify')}
            </button>
          </motion.div>
        </section>
      </div>
    </div>
  );
}
