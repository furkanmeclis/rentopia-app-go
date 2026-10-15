import { CustomerEditPage } from "@/features/customers";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <CustomerEditPage uuid={uuid} />;
}
