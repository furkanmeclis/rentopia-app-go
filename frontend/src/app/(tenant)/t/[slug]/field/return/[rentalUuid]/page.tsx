import { InspectionWizard } from "@/features/inspections";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; rentalUuid: string }>;
}) {
  const { rentalUuid } = await params;
  return <InspectionWizard rentalUuid={rentalUuid} kind="return" />;
}
