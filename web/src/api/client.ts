import type { ErrorBody } from "./types";
import { parseLinkHeader } from "../lib/link";

export class KickflowError extends Error {
  status: number;
  code: string;
  errors?: Record<string, string[]>;
  constructor(status: number, code: string, message: string, errors?: Record<string, string[]>) {
    super(message);
    this.name = "KickflowError";
    this.status = status;
    this.code = code;
    this.errors = errors;
  }
}

export interface Pagination { page: number; perPage: number; total: number; links: Record<string, string> }
let csrfToken: string | null = null;
export function setCsrfToken(token: string | null): void { csrfToken = token; }

function readError(status: number, body: unknown): KickflowError {
  if (body && typeof body === "object" && "code" in body) {
    const error = body as ErrorBody;
    return new KickflowError(status, error.code, error.message ?? `HTTP ${status}`, error.errors);
  }
  return new KickflowError(status, `http_${status}`, `HTTP ${status}`);
}

export async function apiFetch<T>(method: string, path: string, body?: unknown): Promise<T> {
  const upperMethod = method.toUpperCase();
  const headers = new Headers();
  if (body !== undefined) headers.set("Content-Type", "application/json");
  if (csrfToken && ["POST", "PATCH", "DELETE"].includes(upperMethod)) headers.set("X-Punchflow-CSRF", csrfToken);
  const response = await fetch(path, {
    method: upperMethod,
    credentials: "same-origin",
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await response.text();
  let value: unknown;
  if (text) {
    try { value = JSON.parse(text); } catch { value = undefined; }
  }
  if (!response.ok) throw readError(response.status, value);
  return (text ? value : undefined) as T;
}

export function buildQuery(query: Record<string, string | string[] | undefined>): string {
  const pairs: string[] = [];
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined) continue;
    const encodedKey = encodeURIComponent(key);
    if (Array.isArray(value)) for (const item of value) pairs.push(`${encodedKey}[]=${encodeURIComponent(item)}`);
    else pairs.push(`${encodedKey}=${encodeURIComponent(value)}`);
  }
  const encoded = pairs.join("&");
  return encoded ? `?${encoded}` : "";
}

export async function apiList<T>(path: string, query?: Record<string, string | string[]>): Promise<{ items: T[]; pagination: Pagination }> {
  const url = `${path}${buildQuery(query ?? {})}`;
  const response = await fetch(url, { credentials: "same-origin", headers: csrfToken ? { "X-Punchflow-CSRF": csrfToken } : {} });
  const text = await response.text();
  let payload: unknown;
  if (text) { try { payload = JSON.parse(text); } catch { payload = undefined; } }
  if (!response.ok) throw readError(response.status, payload);
  const headers = response.headers;
  return {
    items: (Array.isArray(payload) ? payload : (payload as { items?: T[] } | undefined)?.items ?? []) as T[],
    pagination: {
      page: Number(headers.get("Page") ?? 1),
      perPage: Number(headers.get("Per-Page") ?? 25),
      total: Number(headers.get("Total") ?? 0),
      links: parseLinkHeader(headers.get("Link")),
    },
  };
}

export async function apiUpload(path: string, file: File): Promise<{ signedId: string }> {
  const data = new FormData();
  data.append("file", file);
  const headers = new Headers();
  if (csrfToken) headers.set("X-Punchflow-CSRF", csrfToken);
  const response = await fetch(path, { method: "POST", credentials: "same-origin", headers, body: data });
  const value = await response.json().catch(() => undefined);
  if (!response.ok) throw readError(response.status, value);
  return value as { signedId: string };
}
