"use client";

import { CheckCircle2, FileSignature, PenLine } from "lucide-react";
import { useRef, useState } from "react";

import { EntitySectionCard } from "@/components/entity";
import {
  SignaturePad,
  type SignatureCapture,
  type SignaturePadHandle,
} from "@/components/signature/signature-pad";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { HtmlPreview } from "@/features/contracts/components/html-preview";
import {
  useDocument,
  useDocumentVersion,
} from "@/features/contracts/hooks/use-contracts";
import {
  PhotoThumb,
  mediaUrl,
} from "@/features/inspections/components/photo-capture";
import {
  useMediaLinks,
  useSaveSignature,
} from "@/features/inspections/hooks/use-inspections";
import { inspectionProblemText } from "@/features/inspections/lib/errors";
import type { Inspection } from "@/features/inspections/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Step 7: the customer signs on the screen. When the rental contract still
 * needs the customer's approval, the contract is shown and the same
 * signature records the approval of that exact version (bound to its
 * content hash) — no second signature.
 */
export function SignatureStep({
  ins,
  readOnly,
}: {
  ins: Inspection;
  readOnly: boolean;
}) {
  const { t, locale } = useLocale();
  const pad = useRef<SignaturePadHandle>(null);
  const save = useSaveSignature(ins.uuid);
  const [capture, setCapture] = useState<SignatureCapture | null>(null);
  const [name, setName] = useState(ins.rental.customer.display_name);
  const [resign, setResign] = useState(false);
  const contract = ins.rental.contract;
  const needsContract =
    ins.kind === "handover" && contract.required && !contract.approved;
  const [approveContract, setApproveContract] = useState(true);
  const doc = useDocument(needsContract ? contract.document_uuid : null);
  const current = doc.data?.current_version ?? null;
  const version = useDocumentVersion(
    needsContract ? (contract.document_uuid ?? null) : null,
    current?.uuid ?? null,
  );
  const signature = ins.signature;
  const links = useMediaLinks(
    signature ? ins.uuid : null,
    signature?.uuid ?? "",
  );

  if (signature && !resign) {
    const url = mediaUrl(links.data?.[signature.uuid]);
    return (
      <EntitySectionCard title={t("inspections.signature.title")}>
        <div className="grid gap-3">
          <Alert>
            <CheckCircle2 className="size-4 text-emerald-600" />
            <AlertDescription>
              {t("inspections.signature.signed_by", {
                name: signature.signer_name,
                at: datetime(signature.signed_at, "d MMM yyyy HH:mm", locale),
              })}
            </AlertDescription>
          </Alert>
          <PhotoThumb
            src={url}
            alt={t("inspections.signature.title")}
            className="aspect-[3/1] bg-white"
            fit="contain"
          />
          <p className="text-muted-foreground text-xs">
            {t("inspections.signature.evidence", {
              points: signature.point_count,
              hash: signature.sha256.slice(0, 16),
            })}
          </p>
          {signature.contract_approval ? (
            <Alert>
              <FileSignature className="size-4" />
              <AlertDescription>
                {t("inspections.signature.contract_approved")}
              </AlertDescription>
            </Alert>
          ) : null}
          {!readOnly && !signature.contract_approval ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => setResign(true)}
            >
              <PenLine className="size-4" />
              {t("inspections.signature.resign")}
            </Button>
          ) : null}
        </div>
      </EntitySectionCard>
    );
  }

  const empty = !capture || capture.pointCount < 8;
  return (
    <div className="grid gap-4">
      {needsContract ? (
        <EntitySectionCard title={t("inspections.signature.contract_title")}>
          {version.data ? (
            <HtmlPreview
              html={version.data.html}
              title={t("inspections.signature.contract_title")}
              className="h-[45vh]"
            />
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("inspections.signature.contract_loading")}
            </p>
          )}
          <div className="mt-3 flex items-start gap-3">
            <Checkbox
              id="approve-contract"
              checked={approveContract}
              onCheckedChange={(v) => setApproveContract(v === true)}
              className="mt-0.5 size-5"
            />
            <Label htmlFor="approve-contract" className="text-sm leading-snug">
              {t("inspections.signature.approve_contract")}
            </Label>
          </div>
        </EntitySectionCard>
      ) : null}
      <EntitySectionCard title={t("inspections.signature.title")}>
        {!ins.customer_reviewed_at ? (
          <Alert className="mb-3">
            <AlertDescription>
              {t("inspections.problems.review_first")}
            </AlertDescription>
          </Alert>
        ) : null}
        <div className="grid gap-3">
          <div className="grid gap-1.5">
            <Label htmlFor="signer">{t("inspections.signature.signer")}</Label>
            <Input
              id="signer"
              value={name}
              disabled={readOnly}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <SignaturePad
            ref={pad}
            height={180}
            disabled={readOnly || !ins.customer_reviewed_at}
            onChange={setCapture}
            clearLabel={t("inspections.signature.clear")}
            undoLabel={t("inspections.signature.undo")}
            placeholder={t("inspections.signature.placeholder")}
          />
          <Button
            type="button"
            size="lg"
            disabled={
              readOnly ||
              empty ||
              name.trim().length < 2 ||
              save.isPending ||
              !ins.customer_reviewed_at ||
              (needsContract && approveContract && !current)
            }
            onClick={() => {
              if (!pad.current || !capture) return;
              save.mutate(
                {
                  signer_name: name.trim(),
                  signer_role: "customer",
                  png_base64: pad.current.toPNG(),
                  strokes: {
                    width: capture.width,
                    height: capture.height,
                    strokes: capture.strokes,
                  },
                  device_signed_at: new Date().toISOString(),
                  contract:
                    needsContract && approveContract && current
                      ? {
                          document_uuid: contract.document_uuid as string,
                          version_uuid: current.uuid,
                          content_hash: current.content_hash,
                        }
                      : undefined,
                },
                {
                  onSuccess: () => {
                    setResign(false);
                    appToast.success(t("inspections.signature.saved"));
                  },
                  onError: (e) => {
                    appToast.error(inspectionProblemText(t, e));
                    if (needsContract) void doc.refetch();
                  },
                },
              );
            }}
          >
            <FileSignature className="size-4" />
            {t("inspections.signature.save")}
          </Button>
        </div>
      </EntitySectionCard>
    </div>
  );
}
