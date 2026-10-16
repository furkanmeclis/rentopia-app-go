import { SupplierDetailPage } from "@/features/suppliers";

export default async function Page({
  params,
  searchParams,
}: {
  params: Promise<{ slug: string; uuid: string }>;
  searchParams: Promise<{ tab?: string }>;
}) {
  const { uuid } = await params;
  const { tab } = await searchParams;
  return <SupplierDetailPage uuid={uuid} initialTab={tab} />;
}
