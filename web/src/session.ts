import { createSignal } from "solid-js";
import type { User } from "./api/types";
import { apiFetch, setCsrfToken } from "./api/client";

export interface Session {
  user: User;
  permissions: string[];
  csrfToken: string;
  authMethods: { oidc: boolean; local: boolean };
}

export const [session, setSession] = createSignal<Session | null>(null);

export async function loadSession(): Promise<Session | null> {
  try {
    const value = await apiFetch<Session>("GET", "/auth/session");
    setSession(value);
    setCsrfToken(value.csrfToken);
    return value;
  } catch {
    setSession(null);
    setCsrfToken(null);
    return null;
  }
}
