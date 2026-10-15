import { RatePlanDetailPage } from "@/features/pricing";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <RatePlanDetailPage uuid={uuid} />;
}
