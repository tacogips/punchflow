import { describe, expect, test } from "bun:test";
import type { TicketDetail, TicketStep } from "../api/types";
import { attachmentsOf, availableActions, buildApprovers, inputLabel, inputValueText, routeNominationSteps, sendBackTargets, stepSummary, userLabel, visibleTicketSections } from "./ticket-view";

const user = { id: "u1", name: "User", email: "u@example.test" };
const ticket = (overrides: Record<string, unknown> = {}) => ({
  id: "t1", ticketNumber: "T-1", title: "Request", status: "in_progress", currentStep: 1,
  steps: [{ id: "s1", title: "Approve", stepOrder: 1, actionType: "approve", assignees: [{ user, current: true, completed: false }] }],
  author: { id: "u2" },
  ...overrides,
}) as unknown as TicketDetail;

describe("ticket view rules", () => {
  test("labels users by full name and falls back to email", () => {
    expect(userLabel({ fullName: "Sales Rep", email: "sales@example.test", id: "u1" })).toBe("Sales Rep");
    expect(userLabel({ email: "sales@example.test", id: "u1" })).toBe("sales@example.test");
    expect(userLabel({ firstName: "Rep", lastName: "Sales" })).toBe("Sales Rep");
  });

  test("reads attachments and ignores the unrelated files key", () => {
    expect(attachmentsOf({ attachments: [{ filename: "r.pdf", url: "/files/r.pdf" }], files: [{ filename: "wrong" }] }))
      .toEqual([{ filename: "r.pdf", url: "/files/r.pdf" }]);
    expect(attachmentsOf({ files: [{ filename: "wrong", url: "/wrong" }] })).toEqual([]);
  });

  test("uses form and slip field titles for input labels", () => {
    expect(inputLabel({ formFieldId: "field-1", formField: { title: "Receipt", code: "receipt" } })).toBe("Receipt");
    expect(inputLabel({ slipFieldId: "slip-1", slipField: { title: "Line item", code: "line_item" } })).toBe("Line item");
  });

  test("prefers resolved master and user names for input values", () => {
    expect(inputValueText({ value: null, generalMasterItems: [{ name: "Travel" }, { name: "Meals" }] })).toBe("Travel, Meals");
    expect(inputValueText({ value: null, users: [{ fullName: "Sales Rep" }, { fullName: "Finance Lead" }] })).toBe("Sales Rep, Finance Lead");
  });

  test("groups nominated approvers by route step and removes duplicate ids", () => {
    expect(buildApprovers({ step1: "u1, u2", step2: "" })).toEqual([
      { routeStepCode: "step1", userId: ["u1", "u2"] },
    ]);
    expect(buildApprovers({ step1: "u1 u1, u2" })).toEqual([
      { routeStepCode: "step1", userId: ["u1", "u2"] },
    ]);
  });

  test("returns no nominated approvers for empty input", () => {
    expect(buildApprovers({})).toEqual([]);
  });

  test("reads nomination steps from presenter-shaped route conditions", () => {
    const conditions = [
      { route: { id: "r1", steps: [{ id: "s1", code: "custom", stepType: "author_customizable" }, { id: "s2", code: "fixed", stepType: "user" }] } },
      { route: { id: "r2", steps: [{ id: "s3", code: "custom", stepType: "author_customizable" }, { id: "s4", code: "branch_custom", stepType: "author_customizable" }] } },
      { route: null },
    ];
    expect(routeNominationSteps(conditions).map((step) => step.code)).toEqual(["custom", "branch_custom"]);
  });

  test("ticket form detail renders only sections presenter marks visible", () => {
    const sections = visibleTicketSections([
      { sectionId: "public", visible: true, inputs: [{ formField: { code: "name" }, value: "Ada" }] },
      { sectionId: "conditional", visible: false, inputs: [{ formField: { code: "salary" }, value: "secret" }] },
      { sectionId: "unconditional", inputs: [{ formField: { code: "note" }, value: "ok" }] },
    ]);
    expect(sections.flatMap((section) => (section.inputs as unknown[]).map((input) => inputLabel(input)))).toEqual(["name", "note"]);
  });

  test("current assignee can approve, send back and deny, but not withdraw", () => {
    const actions = availableActions(ticket(), { id: "u1", permissions: [] });
    expect(actions).toContain("approve"); expect(actions).toContain("reject"); expect(actions).toContain("deny");
    expect(actions).not.toContain("withdraw");
  });

  test("author can withdraw in progress and edit or archive a draft", () => {
    expect(availableActions(ticket({ author: user }), { id: "u1", permissions: [] })).toContain("withdraw");
    expect(availableActions(ticket({ status: "draft", author: user }), { id: "u1", permissions: [] })).toEqual(["archive", "edit"]);
  });

  test("confirm action allows approval only", () => {
    const value = ticket({ steps: [{ id: "s1", stepOrder: 1, actionType: "confirm", assignees: [{ user, current: true }] }] });
    const actions = availableActions(value, { id: "u1", permissions: [] });
    expect(actions).toContain("approve"); expect(actions).not.toContain("reject"); expect(actions).not.toContain("deny");
  });

  test("parallel current steps allow actions for a later assigned step", () => {
    const value = ticket({
      steps: [
        { id: "s1", title: "Manager approval", stepOrder: 1, actionType: "approve", assignees: [{ user: { id: "u2" }, current: true, completed: false }] },
        { id: "s2", title: "Finance approval", stepOrder: 1, actionType: "approve", assignees: [{ user: { id: "u1" }, current: true, completed: false }] },
        { id: "s3", title: "Future approval", stepOrder: 2, actionType: "approve", assignees: [{ user, current: false, completed: false }] },
      ],
    });
    expect(availableActions(value, { id: "u1", permissions: [] })).toEqual(["approve", "reject", "deny", "edit"]);
  });

  test("parallel current-step proxy assignment grants actions", () => {
    const value = ticket({
      steps: [
        { id: "s1", title: "Manager approval", stepOrder: 1, actionType: "approve", assignees: [{ user: { id: "u2" }, current: true, completed: false }] },
        { id: "s2", title: "Finance confirmation", stepOrder: 1, actionType: "confirm", assignees: [{ user: { id: "u3" }, proxyUserIds: ["u1"], current: true, completed: false }] },
      ],
    });
    expect(availableActions(value, { id: "u1", permissions: [] })).toEqual(["approve", "edit"]);
  });

  test("sub-status restrictions hide the actions they forbid", () => {
    const restricted = (flags: Record<string, boolean>) => ticket({ author: user, subStatus: { allowWithdrawal: true, allowRejection: true, allowDenial: true, ...flags } });
    expect(availableActions(restricted({ allowWithdrawal: false }), { id: "u1", permissions: [] })).not.toContain("withdraw");
    const approver = availableActions(restricted({ allowRejection: false }), { id: "u1", permissions: [] });
    expect(approver).toContain("approve"); expect(approver).toContain("deny"); expect(approver).not.toContain("reject");
    const noDenial = availableActions(restricted({ allowDenial: false }), { id: "u1", permissions: [] });
    expect(noDenial).toContain("reject"); expect(noDenial).not.toContain("deny");
    expect(availableActions(restricted({}), { id: "u1", permissions: [] })).toEqual(["approve", "reject", "deny", "withdraw", "edit"]);
  });

  test("send back targets the applicant and earlier steps", () => {
    expect(sendBackTargets(ticket({ currentStep: 3 }))).toEqual([
      { to: 0, label: "Applicant" }, { to: 1, label: "Step 1" }, { to: 2, label: "Step 2" },
    ]);
  });

  test("step summary shows approvals against required count", () => {
    const step = { requiredApprovalsNumber: 1, requiredApprovalsPercent: 0, assignees: [{ completed: true }, { completed: false }] } as unknown as TicketStep;
    expect(stepSummary(step)).toContain("1/2 approved (any)");
  });
});
