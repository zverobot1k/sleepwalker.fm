import { AuthGuard } from '@/components/auth-guard';
import { AppShell } from '@/components/app-shell';

export default function ProtectedLayout({ children }: { children: React.ReactNode }) {
  return (
    <AuthGuard>
      <AppShell>{children}</AppShell>
    </AuthGuard>
  );
}
