'use client';

import { Info } from 'lucide-react';

export function InfoBanner({ text }: { text: string }) {
  return (
    <div className="flex gap-3 p-3 rounded-xl border border-violet-400/25 bg-violet-500/10 text-sm text-violet-100">
      <Info className="w-4 h-4 shrink-0 mt-0.5" />
      <p>{text}</p>
    </div>
  );
}
