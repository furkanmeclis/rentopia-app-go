import { createHmac } from "node:crypto";

/** The payment app verifies this signature and re-resolves the destination host. */
export function paymentHandoff(
  claims: { token: string; url: string; organization: string; expires: number },
  secret = process.env.SITE_EMBED_SECRET,
): string {
  if (!secret || secret.length < 32)
    throw new Error("SITE_EMBED_SECRET is required");
  const payload = Buffer.from(JSON.stringify(claims)).toString("base64url");
  const signature = createHmac("sha256", secret)
    .update(`rentopia/widget-payment/v1:${payload}`)
    .digest("base64url");
  return `h1.${payload}.${signature}`;
}
