import type { FormField, SlipItemBody, TicketInputBody, WorkflowDetail } from "../api/types";
export type FieldState = { code: string; fieldType: string; multiple?: boolean; value: unknown };

function fields(workflow: WorkflowDetail) {
  return workflow.sectionList.flatMap((section) => section.sectionType === "form" ? section.formFields ?? [] : []);
}

export function initialFormState(workflow: WorkflowDetail): Record<string, FieldState> {
  return Object.fromEntries(fields(workflow).map((field) => [field.code, { code: field.code, fieldType: field.fieldType, multiple: field.multiple === true, value: null }]));
}

function referenceValue(value: unknown, multiple: boolean): unknown {
  if (multiple) return Array.isArray(value) ? value : value == null || value === "" ? [] : [value];
  return Array.isArray(value) ? value[0] ?? null : value ?? null;
}

function inputFor(state: FieldState, code: string, slip = false): TicketInputBody & { slipFieldCode?: string } {
  const value = state.value;
  const base = slip ? { slipFieldCode: code } : { formFieldCode: code };
  if (state.fieldType === "calculation") return { ...base, value: null };
  if (state.fieldType === "checkbox") return { ...base, value: Array.isArray(value) ? value : value == null || value === "" ? [] : [String(value)] };
  if (state.fieldType === "file") return { ...base, files: Array.isArray(value) ? value.map(String) : value == null ? [] : [String(value)] };
  const normalized = state.fieldType === "datetime" && typeof value === "string" ? value.replace("T", " ") : value;
  if (["master", "general_master", "general_master_item"].includes(state.fieldType)) return { ...base, generalMasterItemId: referenceValue(value, state.multiple === true) };
  if (state.fieldType === "user") return { ...base, userId: referenceValue(value, state.multiple === true) };
  if (state.fieldType === "team") return { ...base, teamId: referenceValue(value, state.multiple === true) };
  if (state.fieldType === "ticket") return { ...base, ticketId: referenceValue(value, state.multiple === true) };
  return { ...base, value: normalized === "" || normalized === undefined ? null : normalized };
}

export function toTicketInputs(workflow: WorkflowDetail, state: Record<string, FieldState>): TicketInputBody[] {
  return fields(workflow).map((field) => inputFor({ ...state[field.code], code: field.code, fieldType: field.fieldType, multiple: field.multiple === true }, field.code));
}

export function toSlipItems(workflow: WorkflowDetail, rows: Record<string, Record<string, FieldState>[]>, fieldFilter: (field: FormField) => boolean = () => true): SlipItemBody[] {
  // slipSectionId selects the section unambiguously (the backend prefers it
  // over the code); every row of the section is sent because slipItems
  // replaces all stored rows.
  return workflow.sectionList.filter((section) => section.sectionType === "slip").flatMap((section) =>
    (rows[section.code ?? section.id] ?? []).map((row) => ({
      slipSectionId: section.id,
      inputs: (section.slipFields ?? []).filter(fieldFilter).map((field) => inputFor(row[field.code] ?? { code: field.code, fieldType: field.fieldType, multiple: field.multiple === true, value: null }, field.code, true) as TicketInputBody & { slipFieldCode: string }),
    })),
  );
}
