import { SegmentDetailPage } from "@/features/fleet";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <SegmentDetailPage uuid={uuid} />;
}
