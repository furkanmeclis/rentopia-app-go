import { RentalDetailPage } from "@/features/rentals";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <RentalDetailPage uuid={uuid} />;
}
