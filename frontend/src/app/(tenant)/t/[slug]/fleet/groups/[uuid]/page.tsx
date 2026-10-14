import { GroupDetailPage } from "@/features/fleet";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <GroupDetailPage uuid={uuid} />;
}
