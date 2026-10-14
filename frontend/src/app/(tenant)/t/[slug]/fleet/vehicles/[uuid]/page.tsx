import { VehicleDetailPage } from "@/features/vehicles";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <VehicleDetailPage uuid={uuid} />;
}
