import { FieldTaskDetailPage } from "@/features/field-tasks";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <FieldTaskDetailPage uuid={uuid} />;
}
