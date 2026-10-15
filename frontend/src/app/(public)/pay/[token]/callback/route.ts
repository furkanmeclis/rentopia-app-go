/**
 * Provider return (iyzico posts the browser here with the checkout token
 * after 3DS). The return is never proof of payment: the body is ignored
 * and the customer goes back to the pay page, which polls the API until a
 * verified result (signed webhook or provider retrieve) exists.
 */
function back(request: Request, token: string) {
  const url = new URL(`/pay/${encodeURIComponent(token)}`, request.url);
  url.searchParams.set("returned", "1");
  return new Response(null, {
    status: 303,
    headers: { Location: url.pathname + url.search },
  });
}

type RouteContext = { params: Promise<{ token: string }> };

export async function POST(request: Request, context: RouteContext) {
  const { token } = await context.params;
  return back(request, token);
}

export async function GET(request: Request, context: RouteContext) {
  const { token } = await context.params;
  return back(request, token);
}
