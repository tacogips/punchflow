import { For, createSignal } from "solid-js";
import type { Accessor, Setter } from "solid-js";
import type { FormField } from "../../api/types";
import type { FieldState } from "../../lib/form-model";
import FormFields from "./FormFields";

export type SlipRows = Record<string, Record<string, FieldState>[]>;
export default function SlipTable(props: {
  code: string;
  title: string;
  fields: FormField[];
  rows: Accessor<SlipRows>;
  setRows: Setter<SlipRows>;
  errors?: Record<string, string[]>;
}) {
  const add = () => props.setRows((rows) => ({ ...rows, [props.code]: [...(rows[props.code] ?? []), Object.fromEntries(props.fields.map((field) => [field.code, { code: field.code, fieldType: field.fieldType, value: null }]))] }));
  const remove = (index: number) => props.setRows((rows) => ({ ...rows, [props.code]: (rows[props.code] ?? []).filter((_, i) => i !== index) }));
  const rowAccessor = (index: number): Accessor<Record<string, FieldState>> => () => props.rows()[props.code]?.[index] ?? {};
  const setRow = (index: number): Setter<Record<string, FieldState>> => (value) => {
    props.setRows((rows) => ({ ...rows, [props.code]: (rows[props.code] ?? []).map((row, i) => i === index ? (typeof value === "function" ? value(row) : value) : row) }));
  };
  return <section class="slip-table"><h3>{props.title}</h3><For each={props.rows()[props.code] ?? []}>{(_, index) => <fieldset class="slip-row">
    <legend>Row {index() + 1}</legend><FormFields fields={props.fields} state={rowAccessor(index())} setState={setRow(index())} errors={props.errors} />
    <button type="button" onClick={() => remove(index())}>Remove row</button>
  </fieldset>}</For><button type="button" onClick={add}>Add row</button></section>;
}
