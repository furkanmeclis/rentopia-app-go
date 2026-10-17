"use client";

import Link from "next/link";
import { useActionState } from "react";
import { useFormStatus } from "react-dom";

import { book, type BookState } from "@/app/actions";
import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/card";
import {
  Checkbox,
  Field,
  Input,
  Label,
  Select,
  Textarea,
} from "@/components/ui/form";

export type BookLabels = Record<string, string>;

function Submit({ labels }: { labels: BookLabels }) {
  const { pending } = useFormStatus();
  return (
    <Button
      type="submit"
      size="lg"
      disabled={pending}
      aria-disabled={pending}
      className="w-full sm:w-auto"
    >
      {pending ? labels["book.submitting"] : labels["book.submit"]}
    </Button>
  );
}

/**
 * Renter, driver and consent form. Posts to the `book` server action
 * (works without JavaScript too); errors come back with the values.
 */
export function BookForm({
  quoteUuid,
  idempotencyKey,
  labels,
}: {
  quoteUuid: string;
  idempotencyKey: string;
  labels: BookLabels;
}) {
  const [state, action] = useActionState<BookState, FormData>(book, {});
  const v = state.values ?? {};
  const err = (k: string) =>
    state.fields?.[k] ? labels[state.fields[k]] : undefined;
  const L = (k: string) => labels[k] ?? k;

  return (
    <form action={action} className="space-y-6" noValidate>
      <input type="hidden" name="quote_uuid" value={quoteUuid} />
      <input type="hidden" name="idempotency_key" value={idempotencyKey} />
      {state.error ? <Alert tone="error">{L(state.error)}</Alert> : null}

      <fieldset className="space-y-4">
        <legend className="mb-2 text-lg font-semibold">
          {L("book.renter")}
        </legend>
        <Field id="type" label={L("book.type")}>
          <Select name="type" defaultValue={v.type || "individual"}>
            <option value="individual">{L("book.type.individual")}</option>
            <option value="corporate">{L("book.type.corporate")}</option>
          </Select>
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field
            id="first_name"
            label={L("book.first_name")}
            error={err("first_name")}
          >
            <Input
              name="first_name"
              autoComplete="given-name"
              defaultValue={v.first_name}
            />
          </Field>
          <Field
            id="last_name"
            label={L("book.last_name")}
            error={err("last_name")}
          >
            <Input
              name="last_name"
              autoComplete="family-name"
              defaultValue={v.last_name}
            />
          </Field>
          <Field
            id="company_title"
            label={L("book.company_title")}
            error={err("company_title")}
          >
            <Input
              name="company_title"
              autoComplete="organization"
              defaultValue={v.company_title}
            />
          </Field>
          <Field
            id="tax_office"
            label={L("book.tax_office")}
            error={err("tax_office")}
          >
            <Input name="tax_office" defaultValue={v.tax_office} />
          </Field>
          <Field id="phone" label={L("book.phone")} error={err("phone")}>
            <Input
              name="phone"
              type="tel"
              autoComplete="tel"
              required
              defaultValue={v.phone}
            />
          </Field>
          <Field id="email" label={L("book.email")} error={err("email")}>
            <Input
              name="email"
              type="email"
              autoComplete="email"
              required
              defaultValue={v.email}
            />
          </Field>
        </div>
      </fieldset>

      <fieldset className="space-y-4">
        <legend className="mb-2 text-lg font-semibold">
          {L("book.driver")}
        </legend>
        <p className="text-muted-foreground text-sm">{L("book.driver_hint")}</p>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field
            id="driver_first_name"
            label={L("book.driver_first_name")}
            error={err("driver_first_name")}
          >
            <Input
              name="driver_first_name"
              defaultValue={v.driver_first_name}
            />
          </Field>
          <Field
            id="driver_last_name"
            label={L("book.driver_last_name")}
            error={err("driver_last_name")}
          >
            <Input name="driver_last_name" defaultValue={v.driver_last_name} />
          </Field>
          <Field
            id="birth_date"
            label={L("book.birth_date")}
            error={err("birth_date")}
          >
            <Input
              name="birth_date"
              type="date"
              autoComplete="bday"
              required
              defaultValue={v.birth_date}
            />
          </Field>
          <Field
            id="licence_number"
            label={L("book.licence_number")}
            error={err("licence_number")}
          >
            <Input
              name="licence_number"
              required
              defaultValue={v.licence_number}
            />
          </Field>
          <Field
            id="licence_issued_on"
            label={L("book.licence_issued_on")}
            error={err("licence_issued_on")}
          >
            <Input
              name="licence_issued_on"
              type="date"
              required
              defaultValue={v.licence_issued_on}
            />
          </Field>
        </div>
        <Field id="note" label={L("book.note")} error={err("note")}>
          <Textarea name="note" maxLength={1000} defaultValue={v.note} />
        </Field>
      </fieldset>

      <fieldset className="space-y-3">
        <legend className="sr-only">{L("book.accept_terms")}</legend>
        <div className="flex items-start gap-2">
          <Checkbox
            id="accept_terms"
            name="accept_terms"
            required
            aria-invalid={err("accept_terms") ? true : undefined}
            className="mt-0.5"
          />
          <Label htmlFor="accept_terms" className="leading-snug font-normal">
            <Link href="/terms" target="_blank" className="underline">
              {L("footer.terms")}
            </Link>
            {" — "}
            {L("book.accept_terms")}
          </Label>
        </div>
        <div className="flex items-start gap-2">
          <Checkbox
            id="accept_privacy"
            name="accept_privacy"
            required
            aria-invalid={err("accept_privacy") ? true : undefined}
            className="mt-0.5"
          />
          <Label htmlFor="accept_privacy" className="leading-snug font-normal">
            <Link href="/privacy" target="_blank" className="underline">
              {L("footer.privacy")}
            </Link>
            {" — "}
            {L("book.accept_privacy")}
          </Label>
        </div>
        {err("accept_terms") || err("accept_privacy") ? (
          <p className="text-sm text-red-600" role="alert">
            {L("book.terms_required")}
          </p>
        ) : null}
      </fieldset>

      <Submit labels={labels} />
    </form>
  );
}
