import { For, Show, createMemo, createResource } from "solid-js";
import type { Accessor, Setter } from "solid-js";
import type { FormField, GeneralMasterItem, TicketWithStep } from "../../api/types";
import { apiFetch, apiList, apiUpload } from "../../api/client";
import type { FieldState } from "../../lib/form-model";
import { fieldInputAllowed, filterMasterItems, type FormActor } from "../../lib/form-ui";

// Conditional display is section-level (kickflow conditions belong to form
// sections); the ticket pages decide which sections to render with
// lib/ticket-form.ts, so fields here are always rendered.
type Loose = Record<string, unknown>;
const obj = (v: unknown): Loose => v && typeof v === "object" ? v as Loose : {};
const str = (v: unknown): string => typeof v === "string" ? v : "";
const arr = (v: unknown): unknown[] => Array.isArray(v) ? v : [];

function FieldControl(props: {
  workflowId: string;
  field: FormField;
  value: unknown;
  actor: FormActor;
  overwritable: boolean;
  form: Accessor<Record<string, FieldState>>;
  fields: FormField[];
  onChange: (value: unknown) => void;
  onFile: (file: File) => Promise<void>;
  onTicketSelect: (field: FormField, ticketID: string) => Promise<void>;
}) {
  const fieldType = props.field.fieldType;
  const editable = fieldInputAllowed(props.field, props.actor, props.overwritable);
  const options = arr(props.field.options ?? props.field.optionList).map((value) => typeof value === "string" ? { value, name: value } : obj(value));
  const generalMasterId = str(obj(props.field.generalMaster).id);
  const [masterItems] = createResource(() => generalMasterId
    ? apiFetch<GeneralMasterItem[]>("GET", `/ui/api/workflows/${encodeURIComponent(props.workflowId)}/form-fields/${encodeURIComponent(props.field.code)}/master-items`)
    : Promise.resolve([]));
  const filteredMasterItems = createMemo(() => filterMasterItems(masterItems() ?? [], props.field, props.form(), props.fields));
  const [tickets] = createResource(() => fieldType === "ticket" ? generalMasterId || props.workflowId : "", async () => {
    const result = await apiList<TicketWithStep>("/v1/tickets", { perPage: "100" });
    const allowed = arr((props.field as unknown as Loose).allowedTicketStatus).map(String);
    const definition = props.field as unknown as Loose;
    const copyWorkflow = str(obj(obj(definition.ticketCopySetting).workflow).id);
    const workflows = copyWorkflow ? [copyWorkflow] : arr(definition.selectableTicketWorkflows).map((workflow) => str(obj(workflow).id));
    return result.items.filter((ticket) => ticket.id !== "" && (!allowed.length || allowed.includes(ticket.status)) && (!workflows.length || workflows.includes(str(obj(ticket.workflow).id))));
  });
  const selected = () => Array.isArray(props.value) ? props.value.map(String).join(", ") : props.value == null ? "" : String(props.value);
  const common = { id: props.field.code, name: props.field.code, disabled: !editable };
  if (fieldType === "checkbox") return <div class="field-control checkbox-group"><For each={options}>{(option) => {
    const value = str(option.value ?? option.code ?? option.id);
    const checked = () => Array.isArray(props.value) ? props.value.map(String).includes(value) : props.value === value;
    return <label><input type="checkbox" disabled={!editable} checked={checked()} onChange={(event) => {
      const next = new Set(Array.isArray(props.value) ? props.value.map(String) : props.value == null ? [] : [String(props.value)]);
      event.currentTarget.checked ? next.add(value) : next.delete(value);
      props.onChange([...next]);
    }} /> {str(option.label ?? option.name ?? value)}</label>;
  }}</For></div>;
  if (fieldType === "text_long") return <textarea {...common} value={selected()} onInput={(event) => props.onChange(event.currentTarget.value)} />;
  if (["pull_down", "radio_button", "master", "general_master"].includes(fieldType)) {
    const items = () => generalMasterId ? filteredMasterItems() : options.map((option) => ({ id: str(option.id ?? option.value), name: str(option.name ?? option.label), code: str(option.code ?? option.value) } as GeneralMasterItem));
    const multiple = obj(props.field).multiple === true;
    const selectedValues = () => Array.isArray(props.value) ? props.value.map(String) : props.value == null || props.value === "" ? [] : [String(props.value)];
    return <select {...common} multiple={multiple} value={multiple ? selectedValues() : selected()} onChange={(event) => props.onChange(multiple ? Array.from(event.currentTarget.selectedOptions, (option) => option.value) : event.currentTarget.value || null)}>
      <Show when={!multiple}><option value="">Select…</option></Show><For each={items()}>{(item) => <option value={item.id}>{item.name}</option>}</For>
    </select>;
  }
  if (fieldType === "ticket" && props.field.multiple !== true) return <select {...common} value={selected()} onChange={(event) => {
    const ticketID = event.currentTarget.value;
    props.onChange(ticketID || null);
    if (ticketID) void props.onTicketSelect(props.field, ticketID);
  }}><option value="">Select ticket…</option><For each={tickets() ?? []}>{(ticket) => <option value={ticket.id}>{ticket.ticketNumber || ticket.title || ticket.id}</option>}</For></select>;
  if (fieldType === "file") return <input {...common} type="file" multiple onChange={async (event) => {
    for (const file of Array.from(event.currentTarget.files ?? [])) await props.onFile(file);
    event.currentTarget.value = "";
  }} />;
  if (fieldType === "calculation") return <output>{selected() || "—"}</output>;
  const multipleReference = obj(props.field).multiple === true && ["user", "team", "ticket"].includes(fieldType);
  return <input {...common} type={fieldType === "date" ? "date" : fieldType === "datetime" ? "datetime-local" : fieldType === "number" || fieldType === "integer" ? "number" : "text"} value={selected()} onInput={(event) => props.onChange(multipleReference ? event.currentTarget.value.split(",").map((value) => value.trim()).filter(Boolean) : event.currentTarget.value)} />;
}

export default function FormFields(props: {
  workflowId: string;
  fields: FormField[];
  state: Accessor<Record<string, FieldState>>;
  setState: Setter<Record<string, FieldState>>;
  actor: FormActor;
  overwritable?: boolean;
  onTicketSelect?: (field: FormField, ticketID: string) => Promise<void>;
  errors?: Record<string, string[]>;
}) {
  const update = (field: FormField, value: unknown) => props.setState((current) => ({
    ...current, [field.code]: { code: field.code, fieldType: field.fieldType, multiple: obj(field).multiple === true, value },
  }));
  const addFile = async (field: FormField, file: File) => {
    const uploaded = await apiUpload("/v1/files", file);
    const current = props.state()[field.code]?.value;
    update(field, [...(Array.isArray(current) ? current.map(String) : current ? [String(current)] : []), uploaded.signedId]);
  };
  return <div class="form-fields"><For each={props.fields.filter((field) => (field as unknown as Loose).hidden !== true)}>{(field) => (
    <div class="form-field"><label for={field.code}>{str((field as unknown as Loose).title) || field.name}{field.required ? " *" : ""}</label>
      <FieldControl workflowId={props.workflowId} field={field} value={props.state()[field.code]?.value} actor={props.actor} overwritable={props.overwritable ?? false} form={props.state} fields={props.fields} onChange={(value) => update(field, value)} onFile={(file) => addFile(field, file)} onTicketSelect={props.onTicketSelect ?? (async () => {})} />
      <Show when={props.errors?.[field.code]}><ul class="field-errors"><For each={props.errors?.[field.code]}>{(error) => <li>{error}</li>}</For></ul></Show>
    </div>
  )}</For></div>;
}
