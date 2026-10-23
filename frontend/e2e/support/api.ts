import { apiBase } from "./env";

/** The `{ success, data, error, meta }` envelope of the Go API. */
export type Envelope<T> = {
  success: boolean;
  data?: T;
  error?: {
    code?: string;
    message?: string;
    details?: { field?: string; code?: string }[];
  };
};

export class ApiFailure extends Error {
  constructor(
    readonly method: string,
    readonly path: string,
    readonly status: number,
    readonly body: unknown,
  ) {
    super(`${method} ${path} → ${status}: ${JSON.stringify(body)}`);
  }
}

export type ApiResult<T> = { status: number; data: T; raw: Envelope<T> };

/**
 * Minimal client of the Go API for provisioning (Node side only). A bearer
 * token from /v1/auth/login; `org` sets X-Organization for /v1/tenant/*.
 */
export class Api {
  constructor(
    private token = "",
    private org = "",
    private headers: Record<string, string> = {},
  ) {}

  static async login(email: string, password: string): Promise<Api> {
    const res = await new Api().call<{ access_token: string }>(
      "POST",
      "/v1/auth/login",
      { email, password },
    );
    return new Api(res.access_token);
  }

  /** A caller with an access token issued earlier (the seed's). */
  static withToken(token: string): Api {
    return new Api(token);
  }

  /** The bearer token (kept in the run fixture: one login per run). */
  get accessToken(): string {
    return this.token;
  }

  /** The same caller inside an organization (slug or uuid). */
  in(org: string): Api {
    return new Api(this.token, org, this.headers);
  }

  /** Extra headers (e.g. a public API bearer with an Origin). */
  with(headers: Record<string, string>): Api {
    return new Api(this.token, this.org, { ...this.headers, ...headers });
  }

  async raw<T>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<ApiResult<T>> {
    const headers: Record<string, string> = {
      Accept: "application/json",
      ...this.headers,
    };
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    if (this.org) headers["X-Organization"] = this.org;
    if (body !== undefined) headers["Content-Type"] = "application/json";
    const res = await fetch(`${apiBase()}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await res.text();
    let raw: Envelope<T> = { success: false };
    try {
      raw = text ? (JSON.parse(text) as Envelope<T>) : raw;
    } catch {
      raw = { success: false, error: { message: text.slice(0, 500) } };
    }
    return { status: res.status, data: raw.data as T, raw };
  }

  /** Calls and fails loudly on anything but 2xx. */
  async call<T>(method: string, path: string, body?: unknown): Promise<T> {
    const res = await this.raw<T>(method, path, body);
    if (res.status < 200 || res.status >= 300) {
      throw new ApiFailure(method, path, res.status, res.raw.error ?? res.raw);
    }
    return res.data;
  }
}
