import { For } from "solid-js";
import { WEBHOOK_EVENT_TYPES } from "../../lib/webhook-view";

export default function EventTypePicker(props: { value: string[]; onChange: (value: string[]) => void }) {
  const toggle = (eventType: string, checked: boolean) => {
    props.onChange(checked ? [...new Set([...props.value, eventType])] : props.value.filter((item) => item !== eventType));
  };
  return <fieldset>
    <legend>Events</legend>
    <p class="muted">Leave all unchecked to receive every non-ping event.</p>
    <div class="check-grid"><For each={WEBHOOK_EVENT_TYPES}>{(eventType) =>
      <label><input type="checkbox" checked={props.value.includes(eventType)} onChange={(event) => toggle(eventType, event.currentTarget.checked)} /> {eventType}</label>
    }</For></div>
  </fieldset>;
}
