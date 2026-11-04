import { afterEach, describe, expect, test } from "bun:test";
import { apiFetch, apiList, buildQuery, KickflowError, setCsrfToken } from "./client";

const originalFetch = globalThis.fetch;
afterEach(() => { globalThis.fetch = originalFetch; setCsrfToken(null); });

describe("API client", () => {
  test("builds kickflow array query parameters", () => {
    expect(buildQuery({ status: ["draft", "completed"], page: "2" })).toBe("?status[]=draft&status[]=completed&page=2");
  });

  test("parses errors and sends CSRF only for mutations", async () => {
    const seen: Request[] = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof Request ? input.url : new URL(input.toString(), "http://localhost");
      seen.push(new Request(url, init));
      return new Response(JSON.stringify({ code: "validation_failed", message: "invalid", errors: { name: ["required"] } }), { status: 422 });
    }) as unknown as typeof fetch;
    setCsrfToken("csrf");
    await expect(apiFetch("POST", "/v1/tickets", {})).rejects.toMatchObject({ name: "KickflowError", status: 422, code: "validation_failed", errors: { name: ["required"] } });
    await expect(apiFetch("GET", "/v1/tickets")).rejects.toBeInstanceOf(KickflowError);
    expect(seen[0].headers.get("X-Punchflow-CSRF")).toBe("csrf");
    expect(seen[1].headers.has("X-Punchflow-CSRF")).toBe(false);
  });

  test("reads pagination headers and Link relations", async () => {
    globalThis.fetch = (async () => new Response(JSON.stringify([{ id: "x" }]), { headers: {
      Page: "2", "Per-Page": "10", Total: "21", Link: '<http://localhost/v1/users?page=3>; rel="next"',
    } })) as unknown as typeof fetch;
    expect(await apiList<{ id: string }>("/v1/users")).toEqual({
      items: [{ id: "x" }], pagination: { page: 2, perPage: 10, total: 21, links: { next: "http://localhost/v1/users?page=3" } },
    });
  });
});
