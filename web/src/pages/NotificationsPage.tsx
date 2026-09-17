import { clsx } from "clsx";
import { CheckCheck } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { useErrorText } from "../components/feedback";
import { Badge, Button, Card, Checkbox, EmptyState, ErrorState, PageHeader, Pagination, Spinner } from "../components/ui";
import { useI18n } from "../i18n";
import { formatWhen } from "../lib/format";
import { useMarkRead, useNotifications } from "../lib/queries";
import type { Notification } from "../lib/types";

// Only links inside the app are followed; anything else stays as text.
function internalPath(url: string | null | undefined): string | null {
  return url && url.startsWith("/") && !url.startsWith("//") ? url : null;
}

export function NotificationsPage() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const errorText = useErrorText();
  const [unreadOnly, setUnreadOnly] = useState(false);
  const [page, setPage] = useState(1);
  const list = useNotifications(unreadOnly, page);
  const markRead = useMarkRead();

  const open = (n: Notification) => {
    if (!n.read_at) markRead.mutate([n.id]);
    const path = internalPath(n.action_url);
    if (path) navigate(path);
  };

  return (
    <>
      <PageHeader
        title={t("notif.title")}
        actions={
          <Button
            variant="secondary"
            icon={<CheckCheck className="size-4" />}
            loading={markRead.isPending && markRead.variables === undefined}
            onClick={() => markRead.mutate(undefined)}
          >
            {t("notif.markAll")}
          </Button>
        }
      />
      <Card padded={false}>
        <div className="border-b border-line p-4">
          <Checkbox
            checked={unreadOnly}
            onChange={(v) => {
              setUnreadOnly(v);
              setPage(1);
            }}
            label={t("notif.unread")}
          />
        </div>
        {list.isLoading ? (
          <Spinner />
        ) : list.error ? (
          <ErrorState message={errorText(list.error)} onRetry={() => void list.refetch()} />
        ) : !list.data || list.data.items.length === 0 ? (
          <EmptyState title={t("notif.empty")} />
        ) : (
          <>
            <ul className="divide-y divide-line">
              {list.data.items.map((n) => (
                <li key={n.id}>
                  <button
                    type="button"
                    onClick={() => open(n)}
                    className={clsx(
                      "flex w-full flex-col gap-1 px-5 py-4 text-left hover:bg-sunken/60",
                      !n.read_at && "bg-brand-soft/40",
                    )}
                  >
                    <span className="flex flex-wrap items-center gap-2">
                      {!n.read_at && <span className="size-2 rounded-full bg-brand" aria-hidden />}
                      <span className="text-sm font-medium text-ink">{n.title}</span>
                      {n.priority === "high" && <Badge tone="bad">!</Badge>}
                      <span className="ml-auto text-xs text-muted tabular">{formatWhen(n.created_at)}</span>
                    </span>
                    <span className="text-sm text-ink-soft">{n.body}</span>
                  </button>
                </li>
              ))}
            </ul>
            <Pagination page={page} pages={list.data.meta.total_pages} onChange={setPage} />
          </>
        )}
      </Card>
    </>
  );
}
