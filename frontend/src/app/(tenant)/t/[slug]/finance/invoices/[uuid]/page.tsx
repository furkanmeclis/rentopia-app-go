import { InvoiceDetailPage } from "@/features/invoices";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <InvoiceDetailPage uuid={uuid} />;
}
