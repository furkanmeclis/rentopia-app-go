/**
 * Provider return of an online deposit pre-authorization (iyzico posts the
 * browser here after 3DS). The return is never proof: the body is ignored,
 * the customer sees a neutral page and staff verify the pre-authorization
 * with a provider retrieve from the panel.
 */
function done(request: Request) {
  const url = new URL("/deposit-preauth/done", request.url);
  return new Response(null, {
    status: 303,
    headers: { Location: url.pathname },
  });
}

export async function POST(request: Request) {
  return done(request);
}

export async function GET(request: Request) {
  return done(request);
}
