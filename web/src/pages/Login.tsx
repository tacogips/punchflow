import { createSignal, onMount, Show } from "solid-js";
import { useNavigate } from "@solidjs/router";
import { apiFetch, KickflowError } from "../api/client";
import { loadSession } from "../session";

export default function Login() {
  const navigate = useNavigate();
  const [email, setEmail] = createSignal("");
  const [error, setError] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  onMount(async () => { if (await loadSession()) navigate("/", { replace: true }); });
  const localLogin = async (event: SubmitEvent) => {
    event.preventDefault();
    setError(""); setBusy(true);
    try {
      await apiFetch("POST", "/auth/local/login", { email: email().trim() });
      if (await loadSession()) navigate("/", { replace: true });
      else setError("The login succeeded, but the session could not be loaded.");
    } catch (cause) {
      setError(cause instanceof KickflowError && cause.status === 404 && cause.code === "endpoint_not_found"
        ? "Local login is disabled on this server."
        : cause instanceof Error ? cause.message : "Unable to sign in.");
    } finally { setBusy(false); }
  };
  return <main class="page login-page"><h1>Sign in to Punchflow</h1>
    <p>Use your configured Keycloak account, or the local development login if enabled.</p>
    <p><a class="button-link" href="/auth/oidc/login?returnTo=%2F">Sign in with Keycloak</a></p>
    <form onSubmit={localLogin} class="stack-form">
      <h2>Local development login</h2>
      <label>Email <input type="email" required value={email()} onInput={(event) => setEmail(event.currentTarget.value)} /></label>
      <button type="submit" disabled={busy()}>{busy() ? "Signing in…" : "Sign in locally"}</button>
    </form>
    <Show when={error()}><p class="error" role="alert">{error()}</p></Show>
  </main>;
}
