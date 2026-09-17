import { clsx } from "clsx";
import { Plus, Settings2, Trash2, Unplug } from "lucide-react";
import { useState } from "react";
import { useSearchParams } from "react-router";
import { useErrorText, useToast } from "../../components/feedback";
import { AccountStatusBadge } from "../../components/status";
import {
  Badge,
  Button,
  Card,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  ErrorState,
  LinkButton,
  PageHeader,
  Spinner,
} from "../../components/ui";
import { useI18n } from "../../i18n";
import { useAuth } from "../../lib/auth";
import { formatWhen } from "../../lib/format";
import { useAccounts, useDisconnect, useGroups, useRemoveGroup } from "../../lib/queries";
import type { TelegramAccount, TelegramGroup } from "../../lib/types";
import { ConnectAccountDialog } from "./ConnectAccountDialog";
import { ConnectGroupDialog } from "./ConnectGroupDialog";

function AccountsTab() {
  const { t } = useI18n();
  const { isManager, can } = useAuth();
  const toast = useToast();
  const errorText = useErrorText();
  const [colleagues, setColleagues] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [leaving, setLeaving] = useState<TelegramAccount | null>(null);
  const accounts = useAccounts(colleagues);
  const disconnect = useDisconnect();

  return (
    <Card
      title={t("tg.accounts")}
      padded={false}
      action={
        <div className="flex items-center gap-3">
          {isManager && <Checkbox checked={colleagues} onChange={setColleagues} label={t("tg.colleagues")} />}
          {can("telegram.connect") && (
            <Button size="sm" icon={<Plus className="size-4" />} onClick={() => setConnecting(true)}>
              {t("tg.connect")}
            </Button>
          )}
        </div>
      }
    >
      {accounts.isLoading ? (
        <Spinner />
      ) : accounts.error ? (
        <ErrorState message={errorText(accounts.error)} onRetry={() => void accounts.refetch()} />
      ) : !accounts.data || accounts.data.length === 0 ? (
        <EmptyState title={t("tg.accountsEmpty")} hint={t("tg.accountsHint")} />
      ) : (
        <ul className="divide-y divide-line">
          {accounts.data.map((a) => (
            <li key={a.id} className="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
              <div className="min-w-0">
                <p className="flex flex-wrap items-center gap-2 text-sm font-medium text-ink">
                  {a.label || a.phone}
                  {a.label && <span className="font-normal text-muted">{a.phone}</span>}
                  <AccountStatusBadge status={a.status} />
                  {!a.mine && <Badge>{t("tg.colleague")}</Badge>}
                </p>
                <p className="mt-1 text-xs text-muted">
                  {a.status === "paused" && a.paused_until
                    ? t("tg.pausedUntil", { time: formatWhen(a.paused_until) })
                    : a.status_reason
                      ? a.status_reason
                      : a.last_used_at
                        ? t("tg.lastUsed", { time: formatWhen(a.last_used_at) })
                        : null}
                </p>
              </div>
              {a.mine && a.status !== "connecting" && (
                <Button variant="danger" size="sm" icon={<Unplug className="size-4" />} onClick={() => setLeaving(a)}>
                  {t("tg.disconnect")}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}

      <ConnectAccountDialog open={connecting} onClose={() => setConnecting(false)} />
      <ConfirmDialog
        open={leaving !== null}
        message={t("tg.disconnectConfirm")}
        confirmLabel={t("tg.disconnect")}
        loading={disconnect.isPending}
        onClose={() => setLeaving(null)}
        onConfirm={() =>
          leaving &&
          disconnect.mutate(leaving.id, {
            onSuccess: () => {
              toast.success(t("tg.disconnected"));
              setLeaving(null);
            },
            onError: (err) => {
              toast.error(errorText(err));
              setLeaving(null);
            },
          })
        }
      />
    </Card>
  );
}

function GroupRow({ group, account, onRemove }: { group: TelegramGroup; account?: TelegramAccount; onRemove: () => void }) {
  const { t } = useI18n();
  const { can } = useAuth();
  const rules = group.rules;
  const routes = group.routes?.length ?? 0;
  return (
    <li className="flex flex-wrap items-start justify-between gap-3 px-5 py-4">
      <div className="min-w-0">
        <p className="flex flex-wrap items-center gap-2 text-sm font-medium text-ink">
          {group.title}
          <Badge>{group.language.toUpperCase()}</Badge>
          {!group.active && <Badge tone="bad">{t("tgg.inactive")}</Badge>}
          {group.active && !group.posting_enabled && <Badge tone="warn">{t("tgg.postingOff")}</Badge>}
          {group.slowmode_seconds > 0 && <Badge tone="warn">{t("tgg.slowmode")}</Badge>}
        </p>
        <p className="mt-1 text-xs text-muted">
          {[
            group.username && `@${group.username}`,
            account && (account.label || account.phone),
            rules && t("tgg.limits", { h: rules.hourly_limit, d: rules.daily_limit, c: rules.cooldown_minutes }),
            routes ? t("tgg.routesCount", { n: routes }) : t("tgg.routesAny"),
            group.last_posted_at && t("tgg.lastPosted", { time: formatWhen(group.last_posted_at) }),
          ]
            .filter(Boolean)
            .join(" · ")}
        </p>
        {group.inactive_reason && <p className="mt-1 text-xs text-bad">{group.inactive_reason}</p>}
      </div>
      {can("telegram.groups.manage") && (
        <div className="flex items-center gap-2">
          <LinkButton
            to={`/telegram/groups/${group.id}`}
            variant="secondary"
            size="sm"
            icon={<Settings2 className="size-4" />}
          >
            {t("tgg.rules")}
          </LinkButton>
          <Button variant="ghost" size="sm" onClick={onRemove} aria-label={t("tgg.remove")}>
            <Trash2 className="size-4" />
          </Button>
        </div>
      )}
    </li>
  );
}

function GroupsTab() {
  const { t } = useI18n();
  const { can } = useAuth();
  const toast = useToast();
  const errorText = useErrorText();
  const groups = useGroups();
  const accounts = useAccounts(true);
  const remove = useRemoveGroup();
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<TelegramGroup | null>(null);

  const mine = (accounts.data ?? []).filter((a) => a.mine && a.status === "active");
  const byId = new Map((accounts.data ?? []).map((a) => [a.id, a]));

  return (
    <Card
      title={t("tg.groups")}
      padded={false}
      action={
        can("telegram.groups.manage") && (
          <Button size="sm" icon={<Plus className="size-4" />} disabled={mine.length === 0} onClick={() => setAdding(true)}>
            {t("tgg.connect")}
          </Button>
        )
      }
    >
      {groups.isLoading ? (
        <Spinner />
      ) : groups.error ? (
        <ErrorState message={errorText(groups.error)} onRetry={() => void groups.refetch()} />
      ) : !groups.data || groups.data.length === 0 ? (
        <EmptyState title={t("tgg.empty")} hint={mine.length === 0 ? t("pub.noAccount") : t("tgg.emptyHint")} />
      ) : (
        <ul className="divide-y divide-line">
          {groups.data.map((g) => (
            <GroupRow key={g.id} group={g} account={byId.get(g.account_id)} onRemove={() => setRemoving(g)} />
          ))}
        </ul>
      )}

      {adding && <ConnectGroupDialog open onClose={() => setAdding(false)} accounts={mine} />}
      <ConfirmDialog
        open={removing !== null}
        message={t("tgg.removeConfirm")}
        confirmLabel={t("tgg.remove")}
        loading={remove.isPending}
        onClose={() => setRemoving(null)}
        onConfirm={() =>
          removing &&
          remove.mutate(removing.id, {
            onSuccess: () => {
              toast.success(t("tgg.removed"));
              setRemoving(null);
            },
            onError: (err) => {
              toast.error(errorText(err));
              setRemoving(null);
            },
          })
        }
      />
    </Card>
  );
}

export function TelegramPage() {
  const { t } = useI18n();
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") === "groups" ? "groups" : "accounts";

  return (
    <>
      <PageHeader title={t("tg.title")} subtitle={t("tg.subtitle")} />
      <div role="tablist" className="mb-4 flex gap-1 border-b border-line">
        {(["accounts", "groups"] as const).map((name) => (
          <button
            key={name}
            role="tab"
            type="button"
            aria-selected={tab === name}
            onClick={() => setParams(name === "accounts" ? {} : { tab: name }, { replace: true })}
            className={clsx(
              "-mb-px border-b-2 px-4 py-2 text-sm font-medium",
              tab === name ? "border-brand text-brand-strong" : "border-transparent text-muted hover:text-ink",
            )}
          >
            {t(name === "accounts" ? "tg.accounts" : "tg.groups")}
          </button>
        ))}
      </div>
      <div role="tabpanel">{tab === "accounts" ? <AccountsTab /> : <GroupsTab />}</div>
    </>
  );
}
