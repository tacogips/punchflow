import { describe, expect, test } from "bun:test";
import { auditExportHref, ticketExportHref, ticketListPath, ticketQuery, type TicketSearchForm } from "./ticket-search";

const empty: TicketSearchForm = { status: [], workflowId: "", ticketNumber: "", keyword: "", createdAtStart: "", createdAtEnd: "" };

describe("ticket search query", () => {
  test("omits empty fields and trims text", () => {
    expect(ticketQuery({ ...empty, status: ["draft"], ticketNumber: " EXP-1 ", keyword: "  offsite lunch ", createdAtStart: "2026-01-01" })).toEqual({
      status: ["draft"], workflowId: undefined, ticketNumber: "EXP-1", q: "offsite lunch", createdAtStart: "2026-01-01", createdAtEnd: undefined,
    });
    expect(ticketQuery({ ...empty, keyword: "   " }).q).toBeUndefined();
  });

  test("uses the keyword endpoint only when a keyword is present", () => {
    expect(ticketListPath(ticketQuery(empty))).toBe("/v1/tickets");
    expect(ticketListPath(ticketQuery({ ...empty, keyword: "lunch" }))).toBe("/ui/api/tickets/search");
  });
});

describe("export links", () => {
  test("ticket export carries the filters, the keyword and the layout but never paging", () => {
    const query = { ...ticketQuery({ ...empty, status: ["draft", "completed"], keyword: "lunch" }), page: "3", perPage: "25" };
    expect(ticketExportHref(query, "detail")).toBe("/ui/api/tickets/export?status[]=draft&status[]=completed&q=lunch&layout=detail");
    expect(ticketExportHref(ticketQuery(empty), "ticket")).toBe("/ui/api/tickets/export?layout=ticket");
  });

  test("encodes the keyword", () => {
    expect(ticketExportHref(ticketQuery({ ...empty, keyword: "a&b 日本" }), "ticket")).toBe(
      "/ui/api/tickets/export?q=a%26b%20%E6%97%A5%E6%9C%AC&layout=ticket",
    );
  });

  test("audit export omits empty filters", () => {
    expect(auditExportHref({ userId: "", createdAtStart: "", createdAtEnd: "" })).toBe("/ui/api/audit-logs/export");
    expect(auditExportHref({ userId: " u1 ", createdAtStart: "2026-01-01", createdAtEnd: "" })).toBe("/ui/api/audit-logs/export?userId=u1&createdAtStart=2026-01-01");
  });
});
