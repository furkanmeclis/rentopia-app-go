"use client";

import { useRouter } from "next/navigation";

import {
  EntitySectionCard,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { routes } from "@/config/routes";
import { useCollectionColumns } from "@/features/finance/components/collection-columns";
import { Amount } from "@/features/finance/components/finance-cells";
import { OpenItemsAllocator } from "@/features/finance/components/open-items-allocator";
import { COLLECTIONS_RESOURCE } from "@/features/finance/constants";
import {
  useCollectionsList,
  useOpenItems,
} from "@/features/finance/hooks/use-finance";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

/**
 * Open items of an account (rent and charges with open / partially paid /
 * paid) and the customer's unallocated credit.
 */
export function OpenItemsCard({ accountUuid }: { accountUuid: string }) {
  const { t } = useLocale();
  const query = useOpenItems(accountUuid, undefined, false);
  const data = query.data;
  return (
    <EntitySectionCard
      title={t("finance.open_items.title")}
      badge={data?.items.filter((i) => i.status !== "paid").length}
      actions={
        data && Number(data.credit) > 0 ? (
          <span className="text-muted-foreground text-sm">
            {t("finance.open_items.credit")}{" "}
            <Amount
              value={data.credit}
              currency={data.currency}
              className="font-medium text-amber-600 dark:text-amber-400"
            />
          </span>
        ) : null
      }
    >
      <OpenItemsAllocator
        items={data?.items ?? []}
        currency={data?.currency ?? "TRY"}
        mode="none"
        amountCents={0}
        manual={{}}
        onManualChange={() => undefined}
        isLoading={query.isLoading}
        persistKey="tenant-finance-open-items-v1"
        readOnly
      />
    </EntitySectionCard>
  );
}

/** The collections of one account (newest first). */
export function AccountCollectionsCard({
  accountUuid,
}: {
  accountUuid: string;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const { slug, organization } = useTenant();
  const listState = useServerListState({
    initialSort: "-collection_date",
    initialPageSize: 10,
    realtime: tenantLiveList(organization, COLLECTIONS_RESOURCE),
  });
  const params = { ...listState.params, account: accountUuid };
  const query = useCollectionsList(params);
  const columns = useCollectionColumns({ showAccount: false });
  const total = query.data?.total ?? 0;
  return (
    <EntitySectionCard title={t("finance.collections.title")} badge={total}>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("finance.load_failed")}
        onRetry={() => void query.refetch()}
        onRowClick={(row) =>
          router.push(routes.tenant.finance.collection(slug, row.uuid))
        }
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("finance.collections.empty")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 10)),
        )}
        state={listState.tableState}
        features={{ persistKey: "tenant-finance-account-collections-v1" }}
      />
    </EntitySectionCard>
  );
}
