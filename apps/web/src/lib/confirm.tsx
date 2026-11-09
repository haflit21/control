import { useCallback, useState, type ReactNode } from 'react';
import { Sheet } from '@maison/ui';

export type ConfirmSpec = { icon: string; tone?: 'warn' | 'crit' | 'heat' | 'off'; title: string; detail?: string; confirmLabel: string; danger?: boolean; run: () => Promise<unknown> | void };

/** One confirmation sheet per page: `ask(spec)` opens it, the element renders it. */
export function useConfirm(): [ReactNode, (spec: ConfirmSpec) => void] {
  const [spec, setSpec] = useState<ConfirmSpec | null>(null);
  const [busy, setBusy] = useState(false);
  const ask = useCallback((s: ConfirmSpec) => setSpec(s), []);
  const cancel = useCallback(() => { if (!busy) setSpec(null); }, [busy]);
  const el = spec ? (
    <Sheet icon={spec.icon} tone={spec.tone} title={spec.title} detail={spec.detail} confirmLabel={spec.confirmLabel} danger={spec.danger} busy={busy} onCancel={cancel}
      onConfirm={async () => { setBusy(true); try { await spec.run(); } catch { /* toast already shown */ } finally { setBusy(false); setSpec(null); } }} />
  ) : null;
  return [el, ask];
}
