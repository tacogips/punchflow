import { describe, expect, test } from "bun:test";
import type { SectionListItem, TicketDetail, WorkflowDetail } from "../api/types";
import { toTicketInputs } from "./form-model";
import { fieldInputAllowed } from "./form-ui";
import {
  buildFormPayload, compareCondition, conditionValues, formStateFromTicket, goSprint, sectionConditionMet,
  defaultFieldValue, sectionViewable, slipRowInSection, slipRowsFromTicket,
} from "./ticket-form";

const always = () => true;

const workflow: WorkflowDetail = {
  id: "wf", name: "Expenses", sectionList: [
    { id: "sec-form", sectionType: "form", title: "Form", formFields: [
      { id: "f-amount", code: "amount", name: "Amount", fieldType: "number" },
      { id: "f-kind", code: "kind", name: "Kind", fieldType: "pull_down" },
      { id: "f-user", code: "owner", name: "Owner", fieldType: "user" },
      { id: "f-team", code: "teams", name: "Teams", fieldType: "team", multiple: true },
      { id: "f-master", code: "category", name: "Category", fieldType: "master" },
      { id: "f-file", code: "document", name: "Document", fieldType: "file" },
      { id: "f-datetime", code: "meeting", name: "Meeting", fieldType: "datetime" },
    ] },
    { id: "sec-items", code: "items", sectionType: "slip", title: "Items", slipFields: [
      { id: "s-desc", code: "desc", name: "Description", fieldType: "text" },
      { id: "s-master", code: "account", name: "Account", fieldType: "master", multiple: true },
      { id: "s-file", code: "receipt", name: "Receipt", fieldType: "file" },
      { id: "s-user", code: "reviewer", name: "Reviewer", fieldType: "user" },
      { id: "s-ticket", code: "related", name: "Related", fieldType: "ticket", multiple: true },
      { id: "s-calc", code: "tax", name: "Tax", fieldType: "calculation" },
      { id: "s-check", code: "approved", name: "Approved", fieldType: "text", approver: true },
    ] },
    { id: "sec-trips", code: "trips", sectionType: "slip", title: "Trips", slipFields: [
      { id: "t-dest", code: "dest", name: "Destination", fieldType: "text" },
    ] },
  ],
};

// Shape returned by GET /v1/tickets/{id} (internal/present/ticket_detail_inputs.go).
const cell = (slipFieldId: string, value: unknown, extras: Record<string, unknown> = {}) => ({ id: `c-${slipFieldId}`, slipFieldId, value, attachments: [], slipField: { id: slipFieldId }, ...extras });
const ticket: TicketDetail = {
  id: "t1", ticketNumber: null, title: "Trip", status: "draft", steps: [], route: null,
  author: { id: "u-author" },
  inputs: [
    { id: "i1", formFieldId: "f-amount", value: "1200", formField: { id: "f-amount", code: "amount" } },
    { id: "i2", formFieldId: "f-kind", value: "item-travel", formField: { id: "f-kind", code: "kind" } },
    { id: "i3", formFieldId: "f-user", value: [{ id: "u-1", name: "Ada" }], users: [{ id: "u-1", name: "Ada" }], formField: { id: "f-user", code: "owner" }, attachments: [] },
    { id: "i4", formFieldId: "f-team", value: [{ id: "team-1", name: "Finance" }, { id: "team-2", name: "Legal" }], teams: [{ id: "team-1", name: "Finance" }, { id: "team-2", name: "Legal" }], formField: { id: "f-team", code: "teams" }, attachments: [] },
    { id: "i5", formFieldId: "f-master", value: [{ id: "gm-1", name: "Travel" }], generalMasterItems: [{ id: "gm-1", name: "Travel" }], formField: { id: "f-master", code: "category" }, attachments: [] },
    { id: "i6", formFieldId: "f-file", value: null, formField: { id: "f-file", code: "document" }, attachments: [{ signedId: "signed-doc" }] },
    { id: "i7", formFieldId: "f-datetime", value: "2026-10-21 10:30", formField: { id: "f-datetime", code: "meeting" }, attachments: [] },
  ],
  slipItems: [
    { id: "r1", slipSectionId: "sec-items", position: 0, inputs: [cell("s-desc", "Taxi"), cell("s-master", [{ id: "acc-1", name: "Travel" }], { generalMasterItems: [{ id: "acc-1", name: "Travel" }] }), cell("s-file", null, { attachments: [{ signedId: "file-1" }, { signedId: "file-2" }] }), cell("s-user", [{ id: "u-reviewer" }], { users: [{ id: "u-reviewer" }] }), cell("s-ticket", [{ id: "t-related" }], { inputTickets: [{ id: "t-related" }] }), cell("s-calc", 100), cell("s-check", "ok")] },
    { id: "r2", slipSectionId: "sec-trips", position: 0, inputs: [cell("t-dest", "Osaka")] },
    { id: "r3", slipSectionId: "sec-items", position: 1, inputs: [cell("s-desc", "Hotel"), cell("s-master", []), cell("s-file", null), cell("s-user", []), cell("s-ticket", []), cell("s-calc", null), cell("s-check", null)] },
  ],
  ticketSections: [
    { id: "sec-form", sectionId: "sec-form", viewable: true },
    { id: "sec-items", sectionId: "sec-items", viewable: true },
    { id: "sec-trips", sectionId: "sec-trips", viewable: true },
  ],
};

describe("restoring a stored ticket", () => {
  test("matches detail rows by slipSectionId and keeps every cell value", () => {
    const rows = slipRowsFromTicket(workflow, ticket);
    expect(rows.items.map((row) => row.desc.value)).toEqual(["Taxi", "Hotel"]);
    expect(rows.items[0].account.value).toEqual(["acc-1"]);
    expect(rows.items[0].receipt.value).toEqual(["file-1", "file-2"]);
    expect(rows.items[0].reviewer.value).toBe("u-reviewer");
    expect(rows.items[0].related.value).toEqual(["t-related"]);
    expect(rows.items[0].approved.value).toBe("ok");
    expect(rows.trips.map((row) => row.dest.value)).toEqual(["Osaka"]);
  });

  test("falls back to the section code when no section ID is present", () => {
    const items = workflow.sectionList[1];
    expect(slipRowInSection({ slipSectionCode: "items" }, items)).toBe(true);
    expect(slipRowInSection({ section: { code: "items" } }, items)).toBe(true);
    expect(slipRowInSection({ slipSectionId: "sec-trips", slipSectionCode: "items" }, items)).toBe(false);
    expect(slipRowInSection({}, items)).toBe(false);
  });

  test("restores form inputs by formFieldId", () => {
    const state = formStateFromTicket(workflow, ticket);
    expect(state.amount.value).toBe("1200");
    expect(state.kind.value).toBe("item-travel");
    expect(state.owner.value).toBe("u-1");
    expect(state.teams.value).toEqual(["team-1", "team-2"]);
    expect(state.category.value).toBe("gm-1");
    expect(state.document.value).toEqual(["signed-doc"]);
    expect(state.meeting.value).toBe("2026-10-21T10:30");
  });

  test("an unchanged author edit round-trip resubmits every stored row", () => {
    const payload = buildFormPayload(workflow, formStateFromTicket(workflow, ticket), slipRowsFromTicket(workflow, ticket), { viewable: always, approver: false });
    expect(payload.inputs).toEqual([
      { formFieldCode: "amount", value: "1200" }, { formFieldCode: "kind", value: "item-travel" },
      { formFieldCode: "owner", userId: "u-1" }, { formFieldCode: "teams", teamId: ["team-1", "team-2"] },
      { formFieldCode: "category", generalMasterItemId: "gm-1" }, { formFieldCode: "document", files: ["signed-doc"] },
      { formFieldCode: "meeting", value: "2026-10-21 10:30" },
    ]);
    expect(payload.slipItems).toEqual([
      { slipSectionId: "sec-items", inputs: [
        { slipFieldCode: "desc", value: "Taxi" }, { slipFieldCode: "account", generalMasterItemId: ["acc-1"] },
        { slipFieldCode: "receipt", files: ["file-1", "file-2"] }, { slipFieldCode: "reviewer", userId: "u-reviewer" },
        { slipFieldCode: "related", ticketId: ["t-related"] }, { slipFieldCode: "tax", value: null },
        { slipFieldCode: "approved", value: "ok" },
      ] },
      { slipSectionId: "sec-items", inputs: [
        { slipFieldCode: "desc", value: "Hotel" }, { slipFieldCode: "account", generalMasterItemId: [] },
        { slipFieldCode: "receipt", files: [] }, { slipFieldCode: "reviewer", userId: null },
        { slipFieldCode: "related", ticketId: [] }, { slipFieldCode: "tax", value: null }, { slipFieldCode: "approved", value: null },
      ] },
      { slipSectionId: "sec-trips", inputs: [{ slipFieldCode: "dest", value: "Osaka" }] },
    ]);
  });

  test("approver mode keeps every row in position but sends only approver fields", () => {
    const payload = buildFormPayload(workflow, formStateFromTicket(workflow, ticket), slipRowsFromTicket(workflow, ticket), { viewable: always, approver: true });
    expect(payload.inputs).toBeUndefined();
    expect(payload.slipItems).toEqual([
      { slipSectionId: "sec-items", inputs: [{ slipFieldCode: "approved", value: "ok" }] },
      { slipSectionId: "sec-items", inputs: [{ slipFieldCode: "approved", value: null }] },
      { slipSectionId: "sec-trips", inputs: [] },
    ]);
  });

  test("approver mode omits slipItems when no detail field is editable", () => {
    const noApprover: WorkflowDetail = { ...workflow, sectionList: [workflow.sectionList[0], workflow.sectionList[2]] };
    const payload = buildFormPayload(noApprover, formStateFromTicket(noApprover, ticket), slipRowsFromTicket(noApprover, ticket), { viewable: always, approver: true });
    expect(payload).toEqual({});
  });
});

describe("browser form permission payloads", () => {
  test("omits hidden, read-only, and other-actor fields", () => {
    const restricted: WorkflowDetail = { ...workflow, sectionList: [{ id: "permissions", sectionType: "form", title: "Permissions", formFields: [
      { id: "hidden", code: "hidden", name: "Hidden", fieldType: "text", author: true, hidden: true },
      { id: "locked", code: "locked", name: "Locked", fieldType: "text", author: true, readonlyOnUi: true },
      { id: "approver", code: "approver", name: "Approver", fieldType: "text", author: false, approver: true },
      { id: "author", code: "author", name: "Author", fieldType: "text", author: true, approver: false },
    ] }] };
    const form = Object.fromEntries(restricted.sectionList[0].formFields!.map((field) => [field.code, { code: field.code, fieldType: field.fieldType, value: "value" }]));
    const applicant = buildFormPayload(restricted, form, {}, { viewable: always, approver: false, editable: (field) => field.fieldType === "calculation" || fieldInputAllowed(field, "applicant") });
    expect(applicant.inputs).toEqual([{ formFieldCode: "author", value: "value" }]);
    const approver = buildFormPayload(restricted, form, {}, { viewable: always, approver: true, editable: (field) => field.fieldType === "calculation" || fieldInputAllowed(field, "approver") });
    expect(approver.inputs).toEqual([{ formFieldCode: "approver", value: "value" }]);
  });
});

describe("new ticket defaults", () => {
  test("applies field defaults while retaining stored values on existing tickets", () => {
    const defaults: WorkflowDetail = { ...workflow, sectionList: [{
      id: "defaults", sectionType: "form", title: "Defaults", formFields: [
        { id: "radio", code: "choice", name: "Choice", fieldType: "radio_button", defaultValue: "approved" },
        { id: "date", code: "today", name: "Today", fieldType: "date", useTodayForDefaultValue: true },
        { id: "master", code: "master", name: "Master", fieldType: "master", generalMaster: { id: "gm" }, defaultGeneralMasterItem: { id: "item-1", name: "Default item" } },
      ],
    }] };
    const fresh = formStateFromTicket(defaults);
    expect(fresh.choice.value).toBe("approved");
    expect(fresh.today.value).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(fresh.master.value).toBe("item-1");

    const existing = { ...ticket, inputs: [{ id: "stored", formFieldId: "radio", value: "rejected", formField: { code: "choice" } }] } as TicketDetail;
    expect(formStateFromTicket(defaults, existing).choice.value).toBe("rejected");
  });

  test("new detail row cells reuse scalar, date, multi-select, and master defaults", () => {
    const today = defaultFieldValue({ id: "d", code: "today", name: "Today", fieldType: "date", useTodayForDefaultValue: true });
    expect(today).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(defaultFieldValue({ id: "r", code: "choice", name: "Choice", fieldType: "radio_button", defaultValue: "yes" })).toBe("yes");
    expect(defaultFieldValue({ id: "m", code: "master", name: "Master", fieldType: "master", defaultGeneralMasterItem: { id: "default-item" } })).toBe("default-item");
    expect(defaultFieldValue({ id: "multi", code: "multi", name: "Multi", fieldType: "user", multiple: true })).toEqual([]);
  });
});

describe("section view restrictions", () => {
  const restricted: SectionListItem = { id: "sec-secret", sectionType: "form", title: "Secret", formFields: [{ id: "f-salary", code: "salary", name: "Salary", fieldType: "number" }], viewRestriction: true, allowViewAuthor: false, sectionViewers: [] } as SectionListItem;
  const restrictedWorkflow: WorkflowDetail = { ...workflow, sectionList: [workflow.sectionList[0], restricted] };

  test("an existing ticket's ticketSections viewable flag is authoritative", () => {
    const stored = { ...ticket, ticketSections: [{ sectionId: "sec-secret", viewable: false }] } as TicketDetail;
    expect(sectionViewable(restricted, stored, { userId: "u-author", permissions: ["ticket_read"] })).toBe(false);
    const allowed = { ...ticket, ticketSections: [{ sectionId: "sec-secret", viewable: true }] } as TicketDetail;
    expect(sectionViewable(restricted, allowed, { userId: "someone" })).toBe(true);
  });

  test("new tickets evaluate the workflow restriction conservatively", () => {
    expect(sectionViewable(workflow.sectionList[0], undefined, {})).toBe(true);
    expect(sectionViewable(restricted, undefined, { userId: "u1" })).toBe(false);
    expect(sectionViewable(restricted, undefined, { userId: "u1", permissions: ["ticket_read"] })).toBe(true);
    expect(sectionViewable({ ...restricted, allowViewAuthor: true } as SectionListItem, undefined, { userId: "u1" })).toBe(true);
    const viewers = (sectionViewers: unknown[]) => ({ ...restricted, sectionViewers }) as SectionListItem;
    expect(sectionViewable(viewers([{ user: { id: "u1" }, team: null, grade: null }]), undefined, { userId: "u1" })).toBe(true);
    expect(sectionViewable(viewers([{ user: { id: "u2" }, team: null, grade: null }]), undefined, { userId: "u1" })).toBe(false);
    expect(sectionViewable(viewers([{ user: null, team: { id: "team-a" }, grade: null }]), undefined, { userId: "u1", teamIds: ["team-a"] })).toBe(true);
    expect(sectionViewable(viewers([{ user: null, team: { id: "team-a" }, grade: { id: "g" } }]), undefined, { userId: "u1", teamIds: ["team-a"] })).toBe(false);
  });

  test("non-viewable sections receive no inputs", () => {
    const state = formStateFromTicket(restrictedWorkflow);
    state.amount.value = "10"; state.salary.value = "999";
    const viewable = (section: SectionListItem) => sectionViewable(section, undefined, { userId: "u1" });
    const payload = buildFormPayload(restrictedWorkflow, state, {}, { viewable, approver: false });
    expect(payload.inputs).toEqual([
      { formFieldCode: "amount", value: "10" }, { formFieldCode: "kind", value: null },
      { formFieldCode: "owner", userId: null }, { formFieldCode: "teams", teamId: [] },
      { formFieldCode: "category", generalMasterItemId: null }, { formFieldCode: "document", files: [] }, { formFieldCode: "meeting", value: null },
    ]);
  });
});

describe("conditional sections", () => {
  const conditional = (combinationType: string, conditionFields: unknown[]): SectionListItem =>
    ({ id: "sec-c", sectionType: "form", title: "Conditional", formFields: [], conditional: true, combinationType, conditionFields }) as SectionListItem;
  const field = (code: string, symbol: string, value: string | null) => ({ id: `${code}-${symbol}`, symbol, value, formField: { id: code, code }, grade: null, team: null, generalMasterItem: null });

  test("sections that are not conditional, or slip sections, always display", () => {
    expect(sectionConditionMet(workflow.sectionList[0], {})).toBe(true);
    expect(sectionConditionMet({ ...workflow.sectionList[1], conditional: true, conditionFields: [field("x", "equal", "y")] } as SectionListItem, {})).toBe(true);
    expect(sectionConditionMet(conditional("all", []), {})).toBe(true);
  });

  test("all requires every condition field and any requires one", () => {
    const fields = [field("amount", "greater_than_or_equal", "1000"), field("kind", "equal", "item-travel")];
    expect(sectionConditionMet(conditional("all", fields), { amount: "1200", kind: "item-travel" })).toBe(true);
    expect(sectionConditionMet(conditional("all", fields), { amount: "999", kind: "item-travel" })).toBe(false);
    expect(sectionConditionMet(conditional("any", fields), { amount: "999", kind: "item-travel" })).toBe(true);
    expect(sectionConditionMet(conditional("any", fields), { amount: "999", kind: "other" })).toBe(false);
    expect(sectionConditionMet(conditional("custom", fields), { amount: "999", kind: "item-travel" })).toBe(false);
  });

  test("resolves general-master thresholds for single and multiple selections", () => {
    const masterCondition = {
      id: "category-condition", symbol: "equal", value: null,
      formField: { id: "category-field", code: "category" }, grade: null, team: null,
      generalMasterItem: { id: "gm-1", code: "travel", name: "Travel" },
    };
    const section = conditional("all", [masterCondition]);
    expect(sectionConditionMet(section, { category: "gm-1" })).toBe(true);
    expect(sectionConditionMet(section, { category: "gm-2" })).toBe(false);

    const multiple = conditional("all", [{ ...masterCondition, symbol: "include" }]);
    expect(sectionConditionMet(multiple, { category: ["gm-2", "gm-1"] })).toBe(true);
    expect(sectionConditionMet(multiple, { category: ["gm-2"] })).toBe(false);
  });

  test("custom expressions use ordered A-J references, parentheses, and backend precedence", () => {
    const fields = [field("amount", "equal", "yes"), field("kind", "equal", "yes"), field("missing", "equal", "yes")];
    const values = { amount: "yes", kind: "no", missing: "no" };
    expect(sectionConditionMet({ ...conditional("custom", fields), combinationExpression: "A || B && C" } as SectionListItem, values)).toBe(true);
    expect(sectionConditionMet({ ...conditional("custom", fields), combinationExpression: "(A || B) && C" } as SectionListItem, values)).toBe(false);
    expect(sectionConditionMet({ ...conditional("custom", fields), combinationExpression: "B || C" } as SectionListItem, values)).toBe(false);
    expect(sectionConditionMet({ ...conditional("custom", fields), combinationExpression: "A &&" } as SectionListItem, values)).toBe(false);
  });

  test("an unsubmitted or unresolved condition field never matches", () => {
    expect(sectionConditionMet(conditional("all", [field("missing", "is_empty", null)]), {})).toBe(false);
    expect(sectionConditionMet(conditional("any", [{ id: "g", symbol: "equal", value: "x", formField: null }]), { x: "x" })).toBe(false);
  });

  test("evaluates the values the editor would submit", () => {
    const state = formStateFromTicket(workflow);
    state.amount.value = "";
    const values = conditionValues(toTicketInputs(workflow, state));
    expect(values).toEqual({ amount: null, kind: null, owner: null, teams: [], category: null, document: null, meeting: null });
    expect(sectionConditionMet(conditional("all", [field("amount", "is_empty", null)]), values)).toBe(true);
    state.kind.value = "item-travel";
    expect(sectionConditionMet(conditional("all", [field("kind", "not_equal", "item-travel")]), conditionValues(toTicketInputs(workflow, state)))).toBe(false);
  });

  test("operators follow the backend comparison semantics", () => {
    expect(goSprint(null)).toBe("<nil>");
    expect(goSprint(["a", 1, true])).toBe("[a 1 true]");
    expect(compareCondition(12, "equal", "12")).toBe(true);
    expect(compareCondition(null, "equal", null)).toBe(true);
    expect(compareCondition(null, "equal", "")).toBe(false);
    expect(compareCondition("a", "not_equal", "b")).toBe(true);
    expect(compareCondition(["a", "b"], "include", "b")).toBe(true);
    expect(compareCondition(["a", "b"], "exclude", "b")).toBe(false);
    expect(compareCondition("b", "include", "b")).toBe(true);
    expect(compareCondition("hello", "contains", "ell")).toBe(true);
    expect(compareCondition([], "is_empty", null)).toBe(true);
    expect(compareCondition("", "is_empty", null)).toBe(true);
    expect(compareCondition("x", "is_not_empty", null)).toBe(true);
    expect(compareCondition("10", "less_than", "9")).toBe(false);
    expect(compareCondition("9", "less_than_or_equal", "9")).toBe(true);
    expect(compareCondition("", "greater_than", "0")).toBe(false);
    expect(compareCondition(" 5", "greater_than", "0")).toBe(false);
    expect(compareCondition(true, "is_true", null)).toBe(true);
    expect(compareCondition("false", "is_false", null)).toBe(true);
    expect(compareCondition("team-a", "descendants_or_equal", "team-a")).toBe(true);
    expect(compareCondition("x", "unknown_operator", "x")).toBe(false);
  });
});
