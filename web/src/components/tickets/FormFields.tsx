import { For, Show, createResource } from "solid-js";
import type { Accessor, Setter } from "solid-js";
import type { FormField, GeneralMasterItem } from "../../api/types";
import { apiList, apiUpload } from "../../api/client";
import type { FieldState } from "../../lib/form-model";

type Loose = Record<string, unknown>;
const obj = (v: unknown): Loose => v && typeof v === "object" ? v as Loose : {};
const str = (v: unknown): string => typeof v === "string" ? v : "";
const arr = (v: unknown): unknown[] => Array.isArray(v) ? v : [];

function visible(field: FormField, state: Record<string, FieldState>): boolean {
  const condition = obj(field.displayCondition ?? field.condition);
  const code = str(condition.fieldCode ?? condition.formFieldCode);
  if (!code) return true;
  const actual = state[code]?.value;
  const expected = condition.value;
  switch (condition.operator ?? condition.conditionType) {
    case "equal": case "equals": return actual === expected;
    case "not_equal": case "notEquals": return actual !== expected;
    case "include": return Array.isArray(actual) ? actual.includes(expected) : String(actual ?? "").includes(String(expected ?? ""));
    default: return true;
  }
}

function FieldControl(props: {
  field: FormField;
  value: unknown;
  onChange: (value: unknown) => void;
  onFile: (file: File) => Promise<void>;
}) {
  const fieldType = props.field.fieldType;
  const options = arr(props.field.options ?? props.field.optionList).map((value) => typeof value === "string" ? { value, name: value } : obj(value));
  const masterId = str(props.field.generalMasterId ?? props.field.masterId);
  const [masterItems] = createResource(() => masterId ? apiList<GeneralMasterItem>(`/v1/generalMasters/${encodeURIComponent(masterId)}/items`).then((r) => r.items) : Promise.resolve([]));
  const selected = () => props.value == null ? "" : String(props.value);
  const common = { id: props.field.code, name: props.field.code, disabled: fieldType === "calculation" };
  if (fieldType === "checkbox") return <div class="field-control checkbox-group"><For each={options}>{(option) => {
    const value = str(option.value ?? option.code ?? option.id);
    const checked = () => Array.isArray(props.value) ? props.value.map(String).includes(value) : props.value === value;
    return <label><input type="checkbox" checked={checked()} onChange={(event) => {
      const next = new Set(Array.isArray(props.value) ? props.value.map(String) : props.value == null ? [] : [String(props.value)]);
      event.currentTarget.checked ? next.add(value) : next.delete(value);
      props.onChange([...next]);
    }} /> {str(option.label ?? option.name ?? value)}</label>;
  }}</For></div>;
  if (fieldType === "text_long") return <textarea {...common} value={selected()} onInput={(event) => props.onChange(event.currentTarget.value)} />;
  if (["pull_down", "radio_button", "master", "general_master"].includes(fieldType)) {
    const items = masterId ? masterItems() ?? [] : options.map((option) => ({ id: str(option.id ?? option.value), name: str(option.name ?? option.label), code: str(option.code ?? option.value) } as GeneralMasterItem));
    return <select {...common} value={selected()} onChange={(event) => props.onChange(event.currentTarget.value || null)}>
      <option value="">Select…</option><For each={items}>{(item) => <option value={item.id}>{item.name}</option>}</For>
    </select>;
  }
  if (fieldType === "file") return <input {...common} type="file" multiple onChange={async (event) => {
    for (const file of Array.from(event.currentTarget.files ?? [])) await props.onFile(file);
    event.currentTarget.value = "";
  }} />;
  if (fieldType === "calculation") return <output>{selected() || "—"}</output>;
  return <input {...common} type={fieldType === "date" ? "date" : fieldType === "datetime" ? "datetime-local" : fieldType === "number" || fieldType === "integer" ? "number" : "text"} value={selected()} onInput={(event) => props.onChange(event.currentTarget.value)} />;
}

export default function FormFields(props: {
  fields: FormField[];
  state: Accessor<Record<string, FieldState>>;
  setState: Setter<Record<string, FieldState>>;
  errors?: Record<string, string[]>;
}) {
  const update = (field: FormField, value: unknown) => props.setState((current) => ({
    ...current, [field.code]: { code: field.code, fieldType: field.fieldType, value },
  }));
  const addFile = async (field: FormField, file: File) => {
    const uploaded = await apiUpload("/v1/files", file);
    const current = props.state()[field.code]?.value;
    update(field, [...(Array.isArray(current) ? current.map(String) : current ? [String(current)] : []), uploaded.signedId]);
  };
  return <div class="form-fields"><For each={props.fields}>{(field) => <Show when={visible(field, props.state())}>
    <div class="form-field"><label for={field.code}>{str((field as unknown as Loose).title) || field.name}{field.required ? " *" : ""}</label>
      <FieldControl field={field} value={props.state()[field.code]?.value} onChange={(value) => update(field, value)} onFile={(file) => addFile(field, file)} />
      <Show when={props.errors?.[field.code]}><ul class="field-errors"><For each={props.errors?.[field.code]}>{(error) => <li>{error}</li>}</For></ul></Show>
    </div>
  </Show>}</For></div>;
}
