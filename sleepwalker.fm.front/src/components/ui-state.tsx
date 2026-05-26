export function LoadingState({ text = 'Loading...' }: { text?: string }) {
  return <div className="p-4 rounded-xl border border-glass-border bg-glass-bg text-muted-foreground">{text}</div>;
}

export function ErrorState({ message }: { message: string }) {
  return <div className="p-4 rounded-xl border border-red-400/30 bg-red-500/10 text-red-200">{message}</div>;
}

export function EmptyState({ text = 'No data available' }: { text?: string }) {
  return <div className="p-4 rounded-xl border border-glass-border bg-glass-bg text-muted-foreground">{text}</div>;
}

export function WarningState({ text }: { text: string }) {
  return <div className="p-3 rounded-xl border border-amber-400/30 bg-amber-500/10 text-amber-100 text-sm">{text}</div>;
}
