import { VehicleDetailPage } from "@/features/vehicles";

export default async function Page({
  params,
  searchParams,
}: {
  params: Promise<{ slug: string; uuid: string }>;
  searchParams: Promise<{ tab?: string | string[] }>;
}) {
  const { uuid } = await params;
  const { tab } = await searchParams;
  // Notification links open a tab directly (`?tab=documents`).
  const initialTab =
    typeof tab === "string" && /^[a-z_-]{1,32}$/.test(tab) ? tab : undefined;
  return <VehicleDetailPage uuid={uuid} initialTab={initialTab} />;
}
