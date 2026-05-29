import { AuthGuard } from '@/components/auth-guard';
import { AppShell } from '@/components/app-shell';
import { TokenRefresh } from '@/components/token-refresh';

export default function ProtectedLayout({ children }: { children: React.ReactNode }) {
  return (
    <AuthGuard>
      <TokenRefresh />
      <AppShell>{children}</AppShell>
    </AuthGuard>
  );
}
