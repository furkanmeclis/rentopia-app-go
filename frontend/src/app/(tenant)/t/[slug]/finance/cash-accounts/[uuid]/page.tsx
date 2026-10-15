import { CashAccountDetailPage } from "@/features/finance";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <CashAccountDetailPage uuid={uuid} />;
}
