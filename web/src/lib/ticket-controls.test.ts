import { describe, expect, test } from "bun:test";
import type { Label } from "../api/types";
import {
  NO_CONTROLS, attachableLabels, customStepBody, customStepProblem, hasAnyControl, labelTextColor,
  normalizeControls, parseUserIds, raiseTargetLabel, subStatusRestrictions, uiTicketPath,
} from "./ticket-controls";

const label = (id: string, name: string, color = "ef4444"): Label => ({ id, name, description: null, color, createdAt: "", updatedAt: "" });

describe("ticket controls", () => {
  test("punchflow-only operations are addressed under /ui/api", () => {
    expect(uiTicketPath("t 1", "/raise")).toBe("/ui/api/tickets/t%201/raise");
  });

  test("normalizes missing or partial control responses to no controls", () => {
    expect(normalizeControls(undefined)).toEqual(NO_CONTROLS);
    expect(normalizeControls({ cancelApproval: true, raiseTargetStepOrders: [2, 3] }))
      .toEqual({ ...NO_CONTROLS, cancelApproval: true, raiseTargetStepOrders: [2, 3] });
    expect(hasAnyControl(NO_CONTROLS)).toBe(false);
    expect(hasAnyControl(normalizeControls({ addCustomStep: true }))).toBe(true);
    // Removing a custom step alone is shown on the step, not in the controls bar.
    expect(hasAnyControl(normalizeControls({ removableCustomStepIds: ["s1"] }))).toBe(false);
  });

  test("parses approver ids separated by commas or spaces without duplicates", () => {
    expect(parseUserIds("u1, u2  u1,,u3")).toEqual(["u1", "u2", "u3"]);
    expect(parseUserIds("  ")).toEqual([]);
  });

  test("validates and builds the custom step request", () => {
    const form = { title: " Legal check ", actionType: "confirm", assignmentType: "user" as const, userIds: "u1,u2", teamCodes: "", requiredApprovalsNumber: "", requiredApprovalsPercent: "", instruction: " read it ", afterStepOrder: "2" };
    expect(customStepProblem(form)).toBeNull();
    expect(customStepBody(form)).toEqual({ title: "Legal check", actionType: "confirm", userIds: ["u1", "u2"], instruction: "read it", afterStepOrder: 2 });
    expect(customStepBody({ ...form, instruction: "", afterStepOrder: "" })).toEqual({ title: "Legal check", actionType: "confirm", userIds: ["u1", "u2"] });
    expect(customStepProblem({ ...form, title: "  " })).toContain("title");
    expect(customStepProblem({ ...form, userIds: "" })).toContain("approver");
    expect(customStepProblem({ ...form, actionType: "unsupported" })).toContain("action");
    expect(customStepBody({ ...form, assignmentType: "team", teamCodes: "sales,finance", requiredApprovalsNumber: "1" })).toEqual({ title: "Legal check", actionType: "confirm", teamCodes: ["sales", "finance"], requiredApprovalsNumber: 1, instruction: "read it", afterStepOrder: 2 });
    const noConfirmation = { ...form, actionType: "none", userIds: "", requiredApprovalsNumber: "", requiredApprovalsPercent: "" };
    expect(customStepProblem(noConfirmation)).toBeNull();
    expect(customStepBody(noConfirmation)).toEqual({ title: "Legal check", actionType: "none", instruction: "read it", afterStepOrder: 2 });
    expect(customStepProblem({ ...form, requiredApprovalsNumber: "1", requiredApprovalsPercent: "50" })).toContain("either");
  });

  test("offers only labels that are not attached yet", () => {
    const all = [label("a", "Urgent"), label("b", "Reviewed")];
    expect(attachableLabels(all, [all[0]!]).map((item) => item.id)).toEqual(["b"]);
    expect(attachableLabels(all, [])).toHaveLength(2);
  });

  test("chooses a readable text color for label backgrounds", () => {
    expect(labelTextColor("ffffff")).toBe("#111827");
    expect(labelTextColor("000000")).toBe("#ffffff");
    expect(labelTextColor("ef4444")).toBe("#ffffff");
    expect(labelTextColor("not-a-color")).toBe("#111827");
  });

  test("lists the actions a sub-status forbids", () => {
    expect(subStatusRestrictions(null)).toEqual([]);
    expect(subStatusRestrictions({ allowWithdrawal: false, allowRejection: true, allowDenial: false })).toEqual(["withdrawal", "denial"]);
    expect(subStatusRestrictions({ allowWithdrawal: true, allowRejection: true, allowDenial: true })).toEqual([]);
  });

  test("names raise targets by step order and title", () => {
    expect(raiseTargetLabel(2, [{ stepOrder: 1, title: "Manager" }, { stepOrder: 2, title: "Finance" }])).toBe("2. Finance");
    expect(raiseTargetLabel(3, [])).toBe("3. Step 3");
  });
});
