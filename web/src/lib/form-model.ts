import type { SlipItemBody, TicketInputBody, WorkflowDetail } from "../api/types";
export type FieldState = { code: string; fieldType: string; value: unknown };

function fields(workflow: WorkflowDetail) {
  return workflow.sectionList.flatMap((section) => section.sectionType === "form" ? section.formFields ?? [] : []);
}

export function initialFormState(workflow: WorkflowDetail): Record<string, FieldState> {
  return Object.fromEntries(fields(workflow).map((field) => [field.code, { code: field.code, fieldType: field.fieldType, value: null }]));
}

function inputFor(state: FieldState, code: string, slip = false): TicketInputBody & { slipFieldCode?: string } {
  const value = state.value;
  const base = slip ? { slipFieldCode: code } : { formFieldCode: code };
  if (state.fieldType === "calculation") return { ...base, value: null };
  if (state.fieldType === "checkbox") return { ...base, value: Array.isArray(value) ? value : value == null || value === "" ? [] : [String(value)] };
  if (state.fieldType === "file") return { ...base, files: Array.isArray(value) ? value.map(String) : value == null ? [] : [String(value)] };
  if (["master", "general_master"].includes(state.fieldType)) return { ...base, generalMasterItemId: value ?? null };
  if (state.fieldType === "user") return { ...base, userId: value ?? null };
  if (state.fieldType === "team") return { ...base, teamId: value ?? null };
  if (state.fieldType === "ticket") return { ...base, ticketId: value ?? null };
  return { ...base, value: value === "" || value === undefined ? null : value };
}

export function toTicketInputs(workflow: WorkflowDetail, state: Record<string, FieldState>): TicketInputBody[] {
  return fields(workflow).map((field) => inputFor(state[field.code] ?? { code: field.code, fieldType: field.fieldType, value: null }, field.code));
}

export function toSlipItems(workflow: WorkflowDetail, rows: Record<string, Record<string, FieldState>[]>): SlipItemBody[] {
  return workflow.sectionList.filter((section) => section.sectionType === "slip").flatMap((section) =>
    (rows[section.code ?? section.id] ?? []).map((row) => ({
      slipSectionCode: section.code ?? section.id,
      inputs: (section.slipFields ?? []).map((field) => inputFor(row[field.code] ?? { code: field.code, fieldType: field.fieldType, value: null }, field.code, true) as TicketInputBody & { slipFieldCode: string }),
    })),
  );
}
