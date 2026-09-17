import { Ban, Check, Clock, Loader2, OctagonAlert, PauseCircle, SkipForward } from "lucide-react";
import type { ReactNode } from "react";
import { useI18n, type MessageKey } from "../i18n";
import type { AccountStatus, AdStatus, JobStatus } from "../lib/types";
import { Badge } from "./ui";

// Every status carries a word and an icon, never colour alone.

const jobTone: Record<JobStatus, { tone: Parameters<typeof Badge>[0]["tone"]; icon: ReactNode }> = {
  pending: { tone: "info", icon: <Clock className="size-3" aria-hidden /> },
  processing: { tone: "brand", icon: <Loader2 className="size-3 animate-spin" aria-hidden /> },
  sent: { tone: "ok", icon: <Check className="size-3" aria-hidden /> },
  failed: { tone: "bad", icon: <OctagonAlert className="size-3" aria-hidden /> },
  skipped: { tone: "neutral", icon: <SkipForward className="size-3" aria-hidden /> },
  rate_limited: { tone: "warn", icon: <PauseCircle className="size-3" aria-hidden /> },
  cancelled: { tone: "neutral", icon: <Ban className="size-3" aria-hidden /> },
};

export function JobStatusBadge({ status }: { status: JobStatus }) {
  const { t } = useI18n();
  const look = jobTone[status] ?? jobTone.pending;
  return (
    <Badge tone={look.tone} icon={look.icon}>
      {t(`status.${status}` as MessageKey)}
    </Badge>
  );
}

const adTone: Record<AdStatus, Parameters<typeof Badge>[0]["tone"]> = {
  draft: "warn",
  ready: "brand",
  publishing: "info",
  published: "ok",
  archived: "neutral",
};

export function AdStatusBadge({ status }: { status: AdStatus }) {
  const { t } = useI18n();
  return <Badge tone={adTone[status] ?? "neutral"}>{t(`adStatus.${status}` as MessageKey)}</Badge>;
}

const accountTone: Record<AccountStatus, Parameters<typeof Badge>[0]["tone"]> = {
  connecting: "info",
  active: "ok",
  paused: "warn",
  restricted: "bad",
  revoked: "bad",
  disabled: "neutral",
};

export function AccountStatusBadge({ status }: { status: AccountStatus }) {
  const { t } = useI18n();
  return <Badge tone={accountTone[status] ?? "neutral"}>{t(`acc.${status}` as MessageKey)}</Badge>;
}
