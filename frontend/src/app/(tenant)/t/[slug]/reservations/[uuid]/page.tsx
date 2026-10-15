import { ReservationDetailPage } from "@/features/reservations";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <ReservationDetailPage uuid={uuid} />;
}
