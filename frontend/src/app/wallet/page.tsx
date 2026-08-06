"use client";

import { RequireAuth } from "@/components/guards";
import { useLocale } from "@/components/providers";
import { Card, CardBody, ErrorState, Skeleton } from "@/components/ui/primitives";
import { useWallet } from "@/hooks/queries";
import { ApiError } from "@/lib/api";
import { formatMoney } from "@/lib/format";

export default function WalletPage() {
  return (
    <RequireAuth permission="payments.read">
      <WalletView />
    </RequireAuth>
  );
}

function WalletView() {
  const { t, locale } = useLocale();
  const wallet = useWallet();

  return (
    <div className="space-y-5">
      <h1 className="text-2xl font-semibold tracking-tight">{t("nav.wallet")}</h1>

      {wallet.isError && (
        <ErrorState
          message={wallet.error instanceof ApiError ? wallet.error.message : t("common.error")}
          onRetry={() => wallet.refetch()}
          retryLabel={t("common.retry")}
        />
      )}

      {wallet.isPending && (
        <div className="grid gap-4 sm:grid-cols-3">
          {Array.from({ length: 3 }, (_, i) => (
            <Skeleton key={i} className="h-24 w-full" />
          ))}
        </div>
      )}

      {wallet.isSuccess && (
        <div className="grid gap-4 sm:grid-cols-3">
          <Stat
            label={t("wallet.balance")}
            value={formatMoney(wallet.data.balance, wallet.data.currency, locale)}
          />
          <Stat
            label={t("wallet.available")}
            value={formatMoney(wallet.data.available, wallet.data.currency, locale)}
            accent
          />
          {/* Held funds are committed to an escrow — spendable balance is
              balance minus this, which is why it is shown alongside. */}
          <Stat
            label={t("wallet.held")}
            value={formatMoney(wallet.data.held_balance, wallet.data.currency, locale)}
          />
        </div>
      )}
    </div>
  );
}

function Stat({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  return (
    <Card>
      <CardBody>
        <p className="text-xs uppercase tracking-wide text-muted-foreground">{label}</p>
        <p className={`tnum mt-1 text-2xl font-semibold ${accent ? "text-accent" : ""}`}>{value}</p>
      </CardBody>
    </Card>
  );
}
