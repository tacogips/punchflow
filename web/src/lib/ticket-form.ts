// Pure helpers for the ticket editor: restoring editor state from a stored
// ticket, evaluating conditional form sections the way the backend does
// (internal/engine/conditions.go), deciding which sections the caller may view
// and enter, and building the PATCH/POST form payload.
import type { FormField, SectionListItem, SlipItemBody, TicketDetail, TicketInputBody, WorkflowDetail } from "../api/types";
import { toSlipItems, toTicketInputs, type FieldState } from "./form-model";

type Loose = Record<string, unknown>;
const obj = (v: unknown): Loose => v && typeof v === "object" ? v as Loose : {};
const arr = (v: unknown): unknown[] => Array.isArray(v) ? v : [];
const str = (v: unknown): string => typeof v === "string" ? v : "";

export type SlipRows = Record<string, Record<string, FieldState>[]>;

/** Key under which a slip section's rows are kept in editor state. */
export const slipKey = (section: SectionListItem): string => section.code ?? section.id;

/** Converts the presenter-shaped value into the identifiers accepted by writes. */
function storedValue(input: Loose | undefined, fieldType: string, multiple: boolean): unknown {
  if (!input) return null;
  if (fieldType === "file") return arr(input.attachments).map((attachment) => str(obj(attachment).signedId)).filter(Boolean);
  let value = input.value;
  if (value == null) value = input.generalMasterItemId ?? input.userId ?? input.teamId ?? input.ticketId ?? null;
  if (["master", "general_master", "general_master_item", "user", "team", "ticket"].includes(fieldType)) {
    const ids = arr(value).map((record) => {
      const item = obj(record);
      return str(item.id ?? item.signedId) || record;
    });
    if (Array.isArray(value)) return multiple ? ids : ids[0] ?? null;
    return multiple ? value == null || value === "" ? [] : [value] : value;
  }
  if (fieldType === "datetime" && typeof value === "string") return value.replace(" ", "T");
  return value ?? null;
}

export function defaultFieldValue(field: FormField): unknown {
  const definition = obj(field);
  if (definition.useTodayForDefaultValue === true && field.fieldType === "date") {
    const today = new Date();
    const month = String(today.getMonth() + 1).padStart(2, "0");
    const day = String(today.getDate()).padStart(2, "0");
    return `${today.getFullYear()}-${month}-${day}`;
  }
  if (field.fieldType === "master" || field.fieldType === "general_master" || field.fieldType === "general_master_item") {
    const id = str(obj(definition.defaultGeneralMasterItem).id);
    return id || (field.multiple === true ? [] : null);
  }
  if (definition.defaultValue !== undefined && definition.defaultValue !== null) return definition.defaultValue;
  return field.multiple === true ? [] : null;
}

/** Restores form-field state from a stored ticket (or empty state for a new one). */
export function formStateFromTicket(workflow: WorkflowDetail, ticket?: TicketDetail): Record<string, FieldState> {
  const state: Record<string, FieldState> = {};
  const inputs = arr(ticket?.inputs).map(obj);
  for (const section of workflow.sectionList ?? []) for (const field of section.formFields ?? []) {
    const input = inputs.find((item) => item.formFieldId === field.id || item.formFieldCode === field.code || obj(item.formField).code === field.code);
    const value = input ? storedValue(input, field.fieldType, field.multiple === true) : defaultFieldValue(field);
    state[field.code] = { code: field.code, fieldType: field.fieldType, multiple: field.multiple === true, value };
  }
  return state;
}

/**
 * Reports whether a stored detail row belongs to the slip section. The backend
 * returns `slipSectionId`; the code forms are accepted as a fallback.
 */
export function slipRowInSection(row: unknown, section: SectionListItem): boolean {
  const item = obj(row);
  const id = str(item.slipSectionId ?? obj(item.section).id ?? obj(item.slipSection).id);
  if (id) return id === section.id;
  const code = str(item.slipSectionCode ?? obj(item.section).code ?? obj(item.slipSection).code);
  return code !== "" && code === slipKey(section);
}

/** Restores every stored detail row of the ticket, keyed by slip section. */
export function slipRowsFromTicket(workflow: WorkflowDetail, ticket?: TicketDetail): SlipRows {
  const rows: SlipRows = {};
  const stored = arr(ticket?.slipItems);
  for (const section of (workflow.sectionList ?? []).filter((item) => item.sectionType === "slip")) {
    rows[slipKey(section)] = stored.filter((row) => slipRowInSection(row, section)).map((row) => {
      const cells = arr(obj(row).inputs).map(obj);
      return Object.fromEntries((section.slipFields ?? []).map((field) => {
        const cell = cells.find((item) => item.slipFieldId === field.id || item.slipFieldCode === field.code || obj(item.slipField).code === field.code);
        return [field.code, { code: field.code, fieldType: field.fieldType, multiple: field.multiple === true, value: storedValue(cell, field.fieldType, field.multiple === true) }];
      }));
    });
  }
  return rows;
}

// ---------------------------------------------------------------------------
// Conditional sections (mirrors pf14SectionVisible / pf14ConditionMatch).

/** Formats a decoded JSON value the way Go's fmt.Sprint does. */
export function goSprint(value: unknown): string {
  if (value === null || value === undefined) return "<nil>";
  if (Array.isArray(value)) return `[${value.map(goSprint).join(" ")}]`;
  if (typeof value === "object") return `map[${Object.keys(value as Loose).sort().map((key) => `${key}:${goSprint((value as Loose)[key])}`).join(" ")}]`;
  return String(value);
}

const goFloat = (value: unknown): number | undefined => {
  const text = goSprint(value);
  return /^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$/.test(text) ? Number(text) : undefined;
};
const isEmptyCollection = (value: unknown): boolean =>
  Array.isArray(value) ? value.length === 0 : value !== null && typeof value === "object" && Object.keys(value).length === 0;

/**
 * Compares a submitted value with a condition threshold. `expected` is the
 * string-or-null threshold emitted by the workflow presenter; the backend
 * compares the stored threshold, which agrees whenever fmt.Sprint does.
 */
export function compareCondition(actual: unknown, symbol: string, expected: unknown): boolean {
  const a = goSprint(actual); const b = goSprint(expected);
  const numeric = (test: (x: number, y: number) => boolean) => {
    const x = goFloat(actual); const y = goFloat(expected);
    return x !== undefined && y !== undefined && test(x, y);
  };
  switch (symbol.toLowerCase()) {
    case "equal": case "equals": case "=": case "==": case "descendants_or_equal": return a === b;
    case "not_equal": case "not_equals": case "!=": case "<>": case "not_descendants_or_equal": return a !== b;
    case "contains": return a.includes(b);
    case "include": case "exclude": {
      const found = Array.isArray(actual) ? actual.some((item) => goSprint(item) === b) : a === b;
      return symbol.toLowerCase() === "exclude" ? !found : found;
    }
    case "is_empty": return actual === null || actual === undefined || a === "" || isEmptyCollection(actual);
    case "is_not_empty": return actual !== null && actual !== undefined && a !== "" && !isEmptyCollection(actual);
    case "greater_than": case ">": return numeric((x, y) => x > y);
    case "greater_than_or_equal": case ">=": return numeric((x, y) => x >= y);
    case "less_than": case "<": return numeric((x, y) => x < y);
    case "less_than_or_equal": case "<=": return numeric((x, y) => x <= y);
    case "is_true": return a === "true";
    case "is_false": return a === "false";
    default: return false;
  }
}

/**
 * Builds the condition-variable map the backend derives from submitted inputs
 * (pf14InputValues): one entry per submitted form field code, holding the
 * value or referenced ID. File fields contribute no value.
 */
export function conditionValues(inputs: TicketInputBody[]): Record<string, unknown> {
  const values: Record<string, unknown> = {};
  for (const input of inputs) {
    if (!input.formFieldCode) continue;
    const item = input as Loose;
    let value: unknown = null;
    for (const key of ["value", "generalMasterItemId", "userId", "teamId", "ticketId"]) if (item[key] !== undefined) value = item[key];
    values[input.formFieldCode] = value;
  }
  return values;
}

function customExpression(expression: string, matches: boolean[]): boolean {
  const tokens = expression.match(/[A-J]|&&|\|\||[()]/g) ?? [];
  if (tokens.join("").replace(/\s/g, "") !== expression.replace(/\s/g, "")) return false;
  let position = 0;
  const parseAtom = (): boolean | undefined => {
    if (tokens[position] === "(") {
      position++;
      const value = parseOr();
      if (tokens[position] !== ")") return undefined;
      position++;
      return value;
    }
    const letter = tokens[position++];
    if (!letter || !/^[A-J]$/.test(letter)) return undefined;
    const index = letter.charCodeAt(0) - 65;
    return index < matches.length ? matches[index] : undefined;
  };
  const parseAnd = (): boolean | undefined => {
    let value = parseAtom();
    if (value === undefined) return undefined;
    while (tokens[position] === "&&") {
      position++;
      const right = parseAtom();
      if (right === undefined) return undefined;
      value = value && right;
    }
    return value;
  };
  const parseOr = (): boolean | undefined => {
    let value = parseAnd();
    if (value === undefined) return undefined;
    while (tokens[position] === "||") {
      position++;
      const right = parseAnd();
      if (right === undefined) return undefined;
      value = value || right;
    }
    return value;
  };
  const result = parseOr();
  return result !== undefined && position === tokens.length && result;
}

/** Evaluates form-section conditions in field order using the backend A-J grammar. */
export function sectionConditionMet(section: SectionListItem, values: Record<string, unknown>): boolean {
  const raw = section as unknown as Loose;
  if (section.sectionType !== "form" || raw.conditional !== true) return true;
  const fields = arr(raw.conditionFields).map(obj);
  if (fields.length === 0) return true;
  const matches = fields.map((field) => {
    const code = str(obj(field.formField).code);
    if (!code || !Object.prototype.hasOwnProperty.call(values, code)) return false;
    // Master thresholds are presented as a resolved record. Their scalar
    // `value` is null, while condition values submitted by the editor are IDs.
    const expected = obj(field.generalMasterItem).id ?? field.value ?? null;
    return compareCondition(values[code], str(field.symbol), expected);
  });
  if (raw.combinationType === "custom") return customExpression(str(raw.combinationExpression), matches);
  const matched = matches.filter(Boolean).length;
  return raw.combinationType === "any" ? matched > 0 : matched === fields.length;
}

// ---------------------------------------------------------------------------
// Section view restrictions.

export interface ViewerContext {
  /** Signed-in user's ID. */
  userId?: string;
  /** Signed-in user's permissions; `ticket_read` sees every section. */
  permissions?: string[];
  /** IDs of the teams the signed-in user belongs to, when known. */
  teamIds?: string[];
}

/**
 * Reports whether the caller may view, and therefore enter, the section. An
 * existing ticket's `ticketSections[].viewable` is authoritative. Otherwise
 * (a new ticket, where the caller is the author) the workflow definition is
 * evaluated conservatively: a team viewer only grants access on a direct team
 * match without a grade restriction, since descendants and grades cannot be
 * resolved client-side. Hidden sections receive no inputs, which the backend
 * accepts, while sending inputs for a blocked section is rejected.
 */
export function sectionViewable(section: SectionListItem, ticket: TicketDetail | undefined, viewer: ViewerContext): boolean {
  const stored = arr(ticket?.ticketSections).map(obj).find((item) => item.sectionId === section.id || item.id === section.id);
  if (stored && typeof stored.viewable === "boolean") return stored.viewable;
  const raw = section as unknown as Loose;
  if (section.sectionType !== "form" || raw.viewRestriction !== true) return true;
  if (viewer.permissions?.includes("ticket_read")) return true;
  if (raw.allowViewAuthor === true && (!ticket || (Boolean(viewer.userId) && obj(ticket.author).id === viewer.userId))) return true;
  return arr(raw.sectionViewers).map(obj).some((item) => {
    const user = obj(item.user); const team = obj(item.team);
    if (item.user) return Boolean(viewer.userId) && user.id === viewer.userId;
    return Boolean(item.team) && !item.grade && (viewer.teamIds ?? []).includes(str(team.id));
  });
}

// ---------------------------------------------------------------------------
// Payload.

/** Fields an approver may enter: approver fields, or every field when the workflow is overwritable. */
export const approverEditable = (workflow: WorkflowDetail, field: FormField): boolean =>
  field.approver === true || (workflow as Loose).overwritable === true;

export interface FormPayload { inputs?: TicketInputBody[]; slipItems?: SlipItemBody[] }

/**
 * Builds the full applicant write payload, retaining all known values and
 * null placeholders for hidden or read-only fields. Approver writes remain
 * differential: only viewable, approver-editable values are sent.
 */
export function buildFormPayload(workflow: WorkflowDetail, form: Record<string, FieldState>, slips: SlipRows, options: { viewable: (section: SectionListItem) => boolean; approver: boolean; editable?: (field: FormField) => boolean }): FormPayload {
  const sectionList = (workflow.sectionList ?? []).filter((section) => options.approver ? options.viewable(section) : true);
  const visible: WorkflowDetail = { ...workflow, sectionList };
  const applicantForm = { ...form };
  const applicantSlips = { ...slips };
  if (!options.approver) for (const section of workflow.sectionList ?? []) {
    if (options.viewable(section)) continue;
    if (section.sectionType === "form") for (const field of section.formFields ?? []) {
      applicantForm[field.code] = { code: field.code, fieldType: field.fieldType, multiple: field.multiple === true, value: null };
    }
    if (section.sectionType === "slip") applicantSlips[slipKey(section)] = (slips[slipKey(section)] ?? []).map(() => Object.fromEntries((section.slipFields ?? []).map((field) => [field.code, { code: field.code, fieldType: field.fieldType, multiple: field.multiple === true, value: null }])));
  }
  const fieldFilter = (field: FormField) => (field as unknown as Loose).hidden !== true && (options.editable?.(field) ?? (!options.approver || approverEditable(workflow, field)));
  const inputs = toTicketInputs(visible, options.approver ? form : applicantForm).filter((input) => {
    const field = sectionList.flatMap((section) => section.formFields ?? []).find((item) => item.code === input.formFieldCode);
    return field !== undefined && (options.approver ? fieldFilter(field) : true);
  });
  const slipItems = toSlipItems(visible, options.approver ? slips : applicantSlips, options.approver ? fieldFilter : () => true);
  if (!options.approver) return { inputs, slipItems };
  const slipEditable = sectionList.some((section) => section.sectionType === "slip" && (section.slipFields ?? []).some(fieldFilter));
  return { ...(inputs.length ? { inputs } : {}), ...(slipEditable ? { slipItems } : {}) };
}
