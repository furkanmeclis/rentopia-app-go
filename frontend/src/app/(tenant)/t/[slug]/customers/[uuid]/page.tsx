import { CustomerDetailPage } from "@/features/customers";

export default async function Page({
  params,
  searchParams,
}: {
  params: Promise<{ slug: string; uuid: string }>;
  searchParams: Promise<{ tab?: string | string[] }>;
}) {
  const { uuid } = await params;
  const { tab } = await searchParams;
  const initialTab =
    typeof tab === "string" && /^[a-z_-]{1,32}$/.test(tab) ? tab : undefined;
  return <CustomerDetailPage uuid={uuid} initialTab={initialTab} />;
}
