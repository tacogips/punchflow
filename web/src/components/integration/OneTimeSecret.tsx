import { createSignal, Show } from "solid-js";

export default function OneTimeSecret(props: { value: string; onClose: () => void }) {
  const [copied, setCopied] = createSignal(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(props.value);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };
  return <section class="notice" aria-live="polite">
    <strong>Copy this secret now. It will not be shown again.</strong>
    <div><code>{props.value}</code></div>
    <button type="button" onClick={copy}>{copied() ? "Copied" : "Copy"}</button>
    <button type="button" onClick={props.onClose}>Done</button>
    <Show when={!copied()}><span class="muted">Clipboard access may require a secure browser context.</span></Show>
  </section>;
}
