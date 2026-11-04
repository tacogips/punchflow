import { Show } from "solid-js";

export function ReadError(props: { error: unknown }) {
  const error = () => props.error as { status?: number; code?: string; message?: string };
  return <section class="notice error" role="alert">
    <Show when={error().status === 403} fallback={<><strong>Could not load data</strong><p>{error().message ?? "The request failed."} {error().code && <code>({error().code})</code>}</p></>}>
      <strong>You do not have permission</strong><p>({error().code ?? "missing_permission"})</p>
    </Show>
  </section>;
}

export default function JsonView(props: { value: unknown; title?: string; open?: boolean }) {
  return <details class="json-view" open={props.open}>
    <summary>{props.title ?? "Raw JSON"}</summary>
    <pre><code>{JSON.stringify(props.value, null, 2)}</code></pre>
  </details>;
}
