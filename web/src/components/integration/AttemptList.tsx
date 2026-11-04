import { For, Show } from "solid-js";
import { formatJst } from "../../lib/format";

type Attempt = {
  id: string;
  statusCode?: number | null;
  startedAt?: string;
  durationMs?: number;
  error?: string | null;
  responseBody?: string | null;
  manual?: boolean;
};

export default function AttemptList(props: { attempts: Attempt[] }) {
  return <div class="table-wrap"><table><thead><tr><th>Attempt</th><th>Result</th><th>Duration</th><th>Started</th><th>Details</th></tr></thead>
    <tbody><For each={props.attempts}>{(attempt, index) => <tr>
      <td>{index() + 1}{attempt.manual ? " (manual)" : ""}</td>
      <td>{attempt.statusCode ?? "Failed"}</td>
      <td>{attempt.durationMs == null ? "—" : `${attempt.durationMs} ms`}</td>
      <td>{formatJst(attempt.startedAt ?? "") || "—"}</td>
      <td><Show when={attempt.error}><span>{attempt.error}</span></Show><Show when={attempt.responseBody}><pre class="excerpt">{attempt.responseBody}</pre></Show></td>
    </tr>}</For></tbody></table>
    <Show when={props.attempts.length === 0}><p class="muted">No delivery attempts yet.</p></Show>
  </div>;
}
