import type { FormField, GeneralMasterItem } from "../api/types";

type Loose = Record<string, unknown>;
const obj = (value: unknown): Loose => value && typeof value === "object" ? value as Loose : {};
const str = (value: unknown): string => value == null ? "" : String(value);
const arr = (value: unknown): unknown[] => Array.isArray(value) ? value : [];

export type FormActor = "applicant" | "approver";

/** Whether a field is displayed and can be entered by the current UI actor. */
export function fieldInputAllowed(field: FormField, actor: FormActor, overwritable = false): boolean {
  const definition = field as unknown as Loose;
  if (definition.hidden === true || definition.readonlyOnUi === true) return false;
  if (field.fieldType === "calculation") return false;
  return actor === "approver" ? field.approver === true || overwritable : definition.author === true;
}

function matches(item: GeneralMasterItem, filter: Loose, value: unknown): boolean {
  const needle = str(value).trim().toLocaleLowerCase();
  if (!needle) return true;
  const type = str(filter.fieldType);
  let values: string[] = [];
  if (type === "name") values = [item.name];
  else if (type === "code") values = [item.code];
  else if (type === "description") values = [str(item.description)];
  else if (type === "custom_field") {
    values = arr(item.inputs).filter((input) => obj(obj(input).field).id === filter.generalMasterFieldId).map((input) => str(obj(input).value));
  } else {
    values = [item.name, item.code, str(item.description), ...arr(item.inputs).map((input) => str(obj(input).value))];
  }
  return values.some((candidate) => candidate.toLocaleLowerCase().includes(needle));
}

/** Applies configured source-field filters to the selectable master items. */
export function filterMasterItems(items: GeneralMasterItem[], field: FormField, form: Record<string, { value: unknown }>, fields: FormField[]): GeneralMasterItem[] {
  const filters = arr((field as unknown as Loose).generalMasterSearchFilters).map(obj);
  const sourceByID = new Map(fields.map((source) => [source.id, form[source.code]?.value]));
  const active = filters.flatMap((filter) => {
    const value = sourceByID.get(str(filter.filterFormFieldId ?? filter.filterSlipFieldId));
    return value == null || value === "" || Array.isArray(value) && value.length === 0 ? [] : [{ filter, value }];
  });
  if (!active.length) return items;
  return items.filter((item) => active.every(({ filter, value }) => matches(item, filter, value)));
}

export interface TicketCopyMapping { fieldType: string; fieldCode: string | null; targetFieldCode: string | null }
export interface TicketCopySetting { workflow?: { id?: string } | null; mappings: TicketCopyMapping[] }

const copyTypes: Record<string, string[]> = {
  ticket_title: ["text", "text_short", "text_long"], ticket_number: ["text", "text_short", "text_long"],
  text: ["text", "text_short", "text_long"], text_short: ["text", "text_short", "text_long"], text_long: ["text_long"], integer: ["integer", "number", "text", "text_short", "text_long"],
  number: ["number", "text", "text_long"], calculation: ["text", "text_long"],
  pull_down: ["pull_down", "radio_button", "text", "text_long"], radio_button: ["radio_button", "pull_down", "text", "text_long"],
  checkbox: ["checkbox"], date: ["date", "text", "text_long"], datetime: ["datetime", "text", "text_long"], file: ["file"],
  master: ["master"], general_master: ["general_master"], user: ["user"], team: ["team"], ticket: ["ticket"],
};

/** Returns true when a configured source kind can populate a destination field. */
export function ticketCopyCompatible(sourceType: string, target: FormField): boolean {
  return (copyTypes[sourceType] ?? []).includes(target.fieldType);
}

/** Resolve a mapping's source data and target field, skipping hidden or noneditable destinations. */
export function ticketCopyValue(mapping: TicketCopyMapping, ticket: Loose, target: FormField, actor: FormActor, overwritable = false): unknown | undefined {
  if (!fieldInputAllowed(target, actor, overwritable)) return undefined;
  if (mapping.fieldType === "ticket_title") return ticketCopyCompatible(mapping.fieldType, target) ? ticket.title : undefined;
  if (mapping.fieldType === "ticket_number") return ticketCopyCompatible(mapping.fieldType, target) ? ticket.ticketNumber : undefined;
  const source = arr(ticket.inputs).map(obj).find((input) => str(input.formFieldCode ?? obj(input.formField).code) === mapping.fieldCode);
  if (!source) return undefined;
  if (obj(source.formField).hidden === true) return undefined;
  const sourceType = str(obj(source.formField).fieldType);
  if (!ticketCopyCompatible(sourceType, target)) return undefined;
  if (target.fieldType === "master" || target.fieldType === "general_master") {
    if (str(obj(obj(source.formField).generalMaster).id) !== str(obj(target.generalMaster).id)) return undefined;
  }
  if (["user", "team", "ticket", "master", "general_master"].includes(target.fieldType) && target.multiple !== obj(source.formField).multiple) return undefined;
  if (target.fieldType === "file") return arr(source.attachments).map((attachment) => str(obj(attachment).signedId)).filter(Boolean);
  const value = source.value ?? source.generalMasterItemId ?? source.userId ?? source.teamId ?? source.ticketId ?? null;
  if (["master", "general_master", "user", "team", "ticket"].includes(target.fieldType)) {
    const ids = (Array.isArray(value) ? value : value == null ? [] : [value]).map((record) => str(obj(record).id) || str(record)).filter(Boolean);
    return target.multiple === true ? ids : ids[0] ?? null;
  }
  if (target.fieldType === "checkbox" || target.fieldType === "pull_down" || target.fieldType === "radio_button") {
    const options = arr((target as unknown as Loose).options).map(String);
    const choices = Array.isArray(value) ? value.map(String) : value == null || value === "" ? [] : [String(value)];
    const accepted = choices.every((choice) => options.includes(choice));
    if (!accepted && target.fieldType === "radio_button") return target.defaultValue ?? null;
    if (!accepted) return undefined;
    return target.fieldType === "checkbox" ? choices : choices[0] ?? null;
  }
  return value;
}
