import { InspectionDetailPage } from "@/features/inspections";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <InspectionDetailPage uuid={uuid} />;
}
