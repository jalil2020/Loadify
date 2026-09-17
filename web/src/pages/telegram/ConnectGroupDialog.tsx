import { Search, Users } from "lucide-react";
import { useMemo, useState } from "react";
import { useErrorText, useToast } from "../../components/feedback";
import { Badge, Button, ErrorState, Input, Modal, Select, Spinner } from "../../components/ui";
import { useI18n } from "../../i18n";
import { formatNumber } from "../../lib/format";
import { useConnectGroup, useDialogs } from "../../lib/queries";
import type { TelegramAccount } from "../../lib/types";

export function ConnectGroupDialog({
  open,
  onClose,
  accounts,
}: {
  open: boolean;
  onClose: () => void;
  accounts: TelegramAccount[];
}) {
  const { t } = useI18n();
  const toast = useToast();
  const errorText = useErrorText();
  const [accountId, setAccountId] = useState("");
  const [query, setQuery] = useState("");
  const [language, setLanguage] = useState("uz");
  const chosen = accountId || accounts[0]?.id || "";
  const dialogs = useDialogs(open ? chosen : undefined);
  const connect = useConnectGroup();

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = dialogs.data ?? [];
    return (q ? list.filter((d) => d.title.toLowerCase().includes(q) || d.username?.toLowerCase().includes(q)) : list)
      .slice()
      .sort((a, b) => Number(b.can_send) - Number(a.can_send) || (b.member_count ?? 0) - (a.member_count ?? 0));
  }, [dialogs.data, query]);

  return (
    <Modal open={open} onClose={onClose} title={t("tgg.connect")} wide>
      <div className="flex flex-col gap-4">
        <div className="grid gap-3 sm:grid-cols-3">
          {accounts.length > 1 && (
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="font-medium text-ink-soft">{t("tgg.chooseAccount")}</span>
              <Select value={chosen} onChange={(e) => setAccountId(e.target.value)}>
                {accounts.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.label ? `${a.label} · ${a.phone}` : a.phone}
                  </option>
                ))}
              </Select>
            </label>
          )}
          <label className="flex flex-col gap-1.5 text-sm">
            <span className="font-medium text-ink-soft">{t("tgg.language")}</span>
            <Select value={language} onChange={(e) => setLanguage(e.target.value)}>
              <option value="uz">O‘zbekcha</option>
              <option value="ru">Русский</option>
              <option value="en">English</option>
            </Select>
          </label>
          <label className="flex flex-col gap-1.5 text-sm sm:col-span-1">
            <span className="font-medium text-ink-soft">{t("common.search")}</span>
            <div className="relative">
              <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted" aria-hidden />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t("tgg.searchDialogs")}
                className="pl-9"
              />
            </div>
          </label>
        </div>

        {dialogs.isLoading ? (
          <Spinner />
        ) : dialogs.error ? (
          <ErrorState message={errorText(dialogs.error)} onRetry={() => void dialogs.refetch()} />
        ) : visible.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted">{t("tgg.dialogsEmpty")}</p>
        ) : (
          <ul className="flex flex-col divide-y divide-line rounded-lg border border-line">
            {visible.map((d) => {
              const busy = connect.isPending && connect.variables?.peer_id === d.peer_id;
              return (
                <li key={d.peer_id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-ink">{d.title}</p>
                    <p className="flex flex-wrap items-center gap-2 text-xs text-muted">
                      {d.username && <span>@{d.username}</span>}
                      {d.member_count !== null && d.member_count !== undefined && (
                        <span className="inline-flex items-center gap-1">
                          <Users className="size-3" aria-hidden />
                          {t("tgg.members", { n: formatNumber(d.member_count) })}
                        </span>
                      )}
                      {d.slowmode_enabled && <Badge tone="warn">{t("tgg.slowmode")}</Badge>}
                    </p>
                  </div>
                  {d.group_id ? (
                    <Badge tone="ok">{t("tgg.connected")}</Badge>
                  ) : !d.can_send ? (
                    <Badge tone="neutral">{t("tgg.cannotSend")}</Badge>
                  ) : (
                    <Button
                      size="sm"
                      loading={busy}
                      disabled={connect.isPending && !busy}
                      onClick={() =>
                        connect.mutate(
                          { account_id: chosen, peer_id: d.peer_id, language },
                          {
                            onSuccess: () => {
                              toast.success(t("tgg.added"));
                              void dialogs.refetch();
                            },
                            onError: (err) => toast.error(errorText(err)),
                          },
                        )
                      }
                    >
                      {t("tgg.add")}
                    </Button>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </Modal>
  );
}
