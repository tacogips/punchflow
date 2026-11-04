import { describe, expect, test } from "bun:test";
import type { AppNotification } from "../api/ui-types";
import { badgeText, kindLabel, notificationHref, notificationSummary, ticketLabel } from "./notification-view";

const base: AppNotification = {
  id: "n1", kind: "ticket_approved", ticketId: "t1", ticketNumber: "EXP-1", ticketTitle: "Dinner", ticketVisible: true,
  commentId: null, actor: { id: "u1", name: "Sales Manager" }, excerpt: "", read: false, readAt: null, createdAt: "2026-01-01T00:00:00.000+09:00",
};

describe("notification view", () => {
  test("summarises lifecycle events with the actor", () => {
    expect(notificationSummary(base)).toBe("Sales Manager approved a ticket");
    expect(notificationSummary({ ...base, kind: "comment_mention" })).toBe("Sales Manager mentioned you in a comment");
    expect(notificationSummary({ ...base, kind: "ticket_updated" })).toBe("Sales Manager edited a ticket");
  });

  test("phrases approval requests as a task", () => {
    expect(notificationSummary({ ...base, kind: "approval_requested" })).toBe("Sales Manager requested approval: needs your approval");
    expect(notificationSummary({ ...base, kind: "approval_requested", actor: null })).toBe("Approval needed: needs your approval");
  });

  test("falls back for a missing actor and unknown kinds", () => {
    expect(notificationSummary({ ...base, actor: null })).toBe("Someone approved a ticket");
    expect(kindLabel("future_kind")).toBe("future kind");
  });

  test("labels tickets by number and title, withholding hidden ones", () => {
    expect(ticketLabel(base)).toBe("EXP-1 Dinner");
    expect(ticketLabel({ ...base, ticketNumber: null })).toBe("Dinner");
    expect(ticketLabel({ ...base, ticketNumber: " ", ticketTitle: null })).toBe("Untitled ticket");
    expect(ticketLabel({ ...base, ticketVisible: false, ticketNumber: null, ticketTitle: null })).toBe("a ticket you can no longer view");
  });

  test("links only to viewable tickets", () => {
    expect(notificationHref(base)).toBe("/tickets/t1");
    expect(notificationHref({ ...base, ticketVisible: false })).toBe("");
  });

  test("formats the unread badge", () => {
    expect(badgeText(0)).toBe("");
    expect(badgeText(-3)).toBe("");
    expect(badgeText(Number.NaN)).toBe("");
    expect(badgeText(7)).toBe("7");
    expect(badgeText(99)).toBe("99");
    expect(badgeText(100)).toBe("99+");
  });
});
