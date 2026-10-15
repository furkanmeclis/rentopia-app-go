"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  CheckCircle2,
  Clock,
  Download,
  FileSignature,
  Loader2,
  MessageSquare,
  ShieldAlert,
  XCircle,
} from "lucide-react";
import { useEffect, useState } from "react";

import { AppWordmark } from "@/components/brand";
import { Loading } from "@/components/common/loading";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { HtmlPreview } from "@/features/contracts/components/html-preview";
import { KIND_LABEL_KEYS } from "@/features/contracts/constants";
import { publicDocumentService } from "@/features/contracts/services/contracts.service";
import type {
  PublicDocument,
  PublicDocumentState,
} from "@/features/contracts/types";
import { ApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const STATE_TITLE_KEYS: Record<PublicDocumentState, string> = {
  pending: "contracts.public.states.pending",
  approved: "contracts.public.states.approved",
  expired: "contracts.public.states.expired",
  revoked: "contracts.public.states.revoked",
  superseded: "contracts.public.states.superseded",
};

const STATE_BODY_KEYS: Record<PublicDocumentState, string> = {
  pending: "contracts.public.bodies.pending",
  approved: "contracts.public.bodies.approved",
  expired: "contracts.public.bodies.expired",
  revoked: "contracts.public.bodies.revoked",
  superseded: "contracts.public.bodies.superseded",
};

function StateIcon({ state }: { state: PublicDocumentState }) {
  switch (state) {
    case "approved":
      return <CheckCircle2 className="size-9 text-emerald-600" />;
    case "expired":
      return <Clock className="text-muted-foreground size-9" />;
    case "revoked":
    case "superseded":
      return <XCircle className="text-destructive size-9" />;
    default:
      return <FileSignature className="text-primary size-9" />;
  }
}

/** Error text for the public routes (generic by design). */
function problemText(t: (key: string) => string, error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.code) {
      case "OTP_INVALID":
        return t("contracts.public.errors.code_invalid");
      case "RATE_LIMITED":
        return t("contracts.public.errors.rate_limited");
      case "CONTENT_CHANGED":
        return t("contracts.public.errors.content_changed");
      case "LINK_UNAVAILABLE":
        return t("contracts.public.errors.unavailable");
      case "CODE_UNAVAILABLE":
        return t("contracts.public.errors.code_unavailable");
    }
  }
  return t("contracts.public.errors.generic");
}

function retryAfter(error: unknown): number {
  if (!(error instanceof ApiError) || error.code !== "RATE_LIMITED") return 0;
  const cooldown = error.details.find((d) => d.code === "cooldown");
  const secs = Number(cooldown?.message ?? 0);
  return Number.isFinite(secs) ? secs : 0;
}

/** The statement shown on the page in Turkish and English (TEC-711). */
function Disclaimer() {
  const { t } = useLocale();
  return (
    <Alert>
      <ShieldAlert className="size-4" />
      <AlertDescription className="text-xs">
        {t("contracts.public.disclaimer")}
      </AlertDescription>
    </Alert>
  );
}

function ApprovalForm({ token, doc }: { token: string; doc: PublicDocument }) {
  const { t, locale } = useLocale();
  const queryClient = useQueryClient();
  const [code, setCode] = useState("");
  const [agreed, setAgreed] = useState(false);
  const [sentTo, setSentTo] = useState<string | null>(null);
  const [wait, setWait] = useState(0);
  const [codeExpires, setCodeExpires] = useState<string | null>(null);

  useEffect(() => {
    if (wait <= 0) return;
    const id = window.setTimeout(() => setWait((n) => n - 1), 1000);
    return () => window.clearTimeout(id);
  }, [wait]);

  const send = useMutation({
    mutationFn: () => publicDocumentService.requestCode(token),
    onSuccess: (out) => {
      setSentTo(out.phone_masked);
      setCodeExpires(out.expires_at);
      const secs = Math.ceil(
        (new Date(out.resend_after).getTime() - Date.now()) / 1000,
      );
      setWait(Math.max(0, Math.min(secs, 120)));
    },
    onError: (error) => {
      const secs = retryAfter(error);
      if (secs > 0) setWait(Math.min(secs, 120));
      if (error instanceof ApiError && error.code === "LINK_UNAVAILABLE") {
        void queryClient.invalidateQueries({
          queryKey: ["public", "document", token],
        });
      }
    },
  });
  const approve = useMutation({
    mutationFn: () =>
      publicDocumentService.approve(token, code.trim(), doc.content_hash),
    onSuccess: (out) => {
      queryClient.setQueryData(["public", "document", token], out);
    },
    onError: (error) => {
      if (
        error instanceof ApiError &&
        (error.code === "LINK_UNAVAILABLE" || error.code === "CONTENT_CHANGED")
      ) {
        void queryClient.invalidateQueries({
          queryKey: ["public", "document", token],
        });
      }
    },
  });
  const problem = approve.error ?? send.error;
  const validCode = /^\d{6}$/.test(code.trim());

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">
          {t("contracts.public.approve_title")}
        </CardTitle>
        <CardDescription>
          {t("contracts.public.approve_description", {
            phone: doc.phone_masked ?? "",
          })}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {problem ? (
          <Alert variant="destructive">
            <AlertTriangle className="size-4" />
            <AlertDescription>{problemText(t, problem)}</AlertDescription>
          </Alert>
        ) : null}
        <div className="flex items-start gap-3">
          <Checkbox
            id="remote-approve-read"
            checked={agreed}
            onCheckedChange={(v) => setAgreed(v === true)}
            className="mt-0.5"
          />
          <Label
            htmlFor="remote-approve-read"
            className="leading-snug font-normal"
          >
            {t("contracts.public.read_confirm", {
              reference: doc.reference,
              version: doc.version_number,
            })}
          </Label>
        </div>
        <Button
          type="button"
          variant={sentTo ? "outline" : "default"}
          className="w-full"
          disabled={send.isPending || wait > 0}
          onClick={() => send.mutate()}
        >
          {send.isPending ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            <MessageSquare className="size-4" />
          )}
          {wait > 0
            ? t("contracts.public.resend_in", { seconds: wait })
            : sentTo
              ? t("contracts.public.resend")
              : t("contracts.public.send_code")}
        </Button>
        {sentTo ? (
          <p className="text-muted-foreground text-sm" role="status">
            {t("contracts.public.code_sent", {
              phone: sentTo,
              until: codeExpires ? datetime(codeExpires, "HH:mm", locale) : "",
            })}
          </p>
        ) : null}
        <div className="space-y-2">
          <Label htmlFor="remote-approve-code">
            {t("contracts.public.code_label")}
          </Label>
          <Input
            id="remote-approve-code"
            value={code}
            onChange={(e) =>
              setCode(e.target.value.replace(/\D/g, "").slice(0, 6))
            }
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]*"
            maxLength={6}
            placeholder="••••••"
            className="text-center font-mono text-lg tracking-[0.5em]"
          />
        </div>
      </CardContent>
      <CardFooter>
        <Button
          type="button"
          className="w-full"
          disabled={!agreed || !validCode || approve.isPending}
          onClick={() => approve.mutate()}
        >
          {approve.isPending ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            <CheckCircle2 className="size-4" />
          )}
          {t("contracts.public.approve")}
        </Button>
      </CardFooter>
    </Card>
  );
}

/**
 * Public remote approval page (`/d/{token}`, TEC-711): the stored version
 * in a fully sandboxed iframe, the masked number, "Kod gönder" and the
 * code. The approval binds to the content hash shown here; after it the
 * approved PDF downloads through the API with the same token.
 */
export function RemoteApprovalPage({ token }: { token: string }) {
  const { t, locale } = useLocale();
  const query = useQuery({
    queryKey: ["public", "document", token],
    queryFn: () => publicDocumentService.get(token),
    retry: false,
  });

  let body: React.ReactNode;
  if (query.isLoading) {
    body = <Loading />;
  } else if (query.isError || !query.data) {
    body = (
      <Alert variant="destructive">
        <AlertTitle>{t("contracts.public.not_found_title")}</AlertTitle>
        <AlertDescription>
          {query.error instanceof ApiError &&
          query.error.code === "RATE_LIMITED"
            ? t("contracts.public.errors.rate_limited")
            : t("contracts.public.not_found_body")}
        </AlertDescription>
      </Alert>
    );
  } else {
    const doc = query.data;
    body = (
      <>
        <Card>
          <CardHeader className="items-center text-center">
            <StateIcon state={doc.state} />
            <CardTitle className="text-xl">
              {t(STATE_TITLE_KEYS[doc.state])}
            </CardTitle>
            <CardDescription>{t(STATE_BODY_KEYS[doc.state])}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <div className="flex justify-between gap-4">
              <span className="text-muted-foreground">
                {t("contracts.public.organization")}
              </span>
              <span className="text-end font-medium">
                {doc.organization_name}
              </span>
            </div>
            <div className="flex justify-between gap-4">
              <span className="text-muted-foreground">
                {t("contracts.public.document")}
              </span>
              <span className="text-end">
                {t(KIND_LABEL_KEYS[doc.kind])} · {doc.reference} · v
                {doc.version_number}
              </span>
            </div>
            <div className="flex justify-between gap-4">
              <span className="text-muted-foreground">
                {t("contracts.public.signer")}
              </span>
              <span className="text-end">{doc.signer_name}</span>
            </div>
            {doc.state === "pending" ? (
              <div className="flex justify-between gap-4">
                <span className="text-muted-foreground">
                  {t("contracts.public.valid_until")}
                </span>
                <span className="text-end tabular-nums">
                  {datetime(doc.expires_at, "d MMM yyyy HH:mm", locale)}
                </span>
              </div>
            ) : null}
            {doc.approved_at ? (
              <div className="flex justify-between gap-4">
                <span className="text-muted-foreground">
                  {t("contracts.public.approved_at")}
                </span>
                <span className="text-end tabular-nums">
                  {datetime(doc.approved_at, "d MMM yyyy HH:mm", locale)}
                </span>
              </div>
            ) : null}
            <div className="flex justify-between gap-4">
              <span className="text-muted-foreground">
                {t("contracts.public.hash")}
              </span>
              <code className="text-end text-xs break-all">
                {doc.content_hash.slice(0, 16)}…
              </code>
            </div>
          </CardContent>
          {doc.state === "approved" && doc.pdf_available ? (
            <CardFooter>
              <Button asChild className="w-full">
                <a
                  href={publicDocumentService.pdfUrl(token)}
                  download
                  rel="noreferrer"
                >
                  <Download className="size-4" />
                  {t("contracts.public.download_pdf")}
                </a>
              </Button>
            </CardFooter>
          ) : null}
        </Card>
        {doc.html ? (
          <HtmlPreview
            html={doc.html}
            title={t("contracts.public.document")}
            className="h-[65svh] md:h-[75vh]"
          />
        ) : null}
        {doc.state === "pending" ? (
          <ApprovalForm token={token} doc={doc} />
        ) : null}
      </>
    );
  }

  return (
    <div className="bg-muted relative flex min-h-svh flex-col items-center px-4 py-6 md:p-10">
      <div className="absolute top-4 right-4 flex items-center gap-1">
        <LocaleSwitch />
        <ThemeSwitch />
      </div>
      <div className="flex w-full max-w-3xl flex-col gap-4">
        <div className="self-center pt-2 pb-2">
          <AppWordmark />
        </div>
        {body}
        <Disclaimer />
      </div>
    </div>
  );
}
