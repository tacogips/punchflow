import { buildQuery } from "../api/client";

export type TicketQuery = Record<string, string | string[] | undefined>;
export type ExportLayout = "ticket" | "detail";

export interface TicketSearchForm {
  status: string[];
  workflowId: string;
  ticketNumber: string;
  keyword: string;
  createdAtStart: string;
  createdAtEnd: string;
}

export const SEARCH_PATH = "/ui/api/tickets/search";
export const LIST_PATH = "/v1/tickets";
export const TICKET_EXPORT_PATH = "/ui/api/tickets/export";
export const AUDIT_EXPORT_PATH = "/ui/api/audit-logs/export";

/** Structured filters plus the optional free-text keyword (`q`). Empty values are omitted. */
export function ticketQuery(form: TicketSearchForm): TicketQuery {
  return {
    status: form.status,
    workflowId: form.workflowId || undefined,
    ticketNumber: form.ticketNumber.trim() || undefined,
    q: form.keyword.trim() || undefined,
    createdAtStart: form.createdAtStart || undefined,
    createdAtEnd: form.createdAtEnd || undefined,
  };
}

/**
 * The keyword search endpoint is only used when a keyword is present, so plain
 * structured searches keep going through the kickflow-compatible /v1 list.
 */
export function ticketListPath(query: TicketQuery): string {
  return typeof query.q === "string" && query.q !== "" ? SEARCH_PATH : LIST_PATH;
}

/** Download link for the complete result of the current filters and keyword. */
export function ticketExportHref(query: TicketQuery, layout: ExportLayout): string {
  const { page: _page, perPage: _perPage, ...filters } = query;
  return `${TICKET_EXPORT_PATH}${buildQuery({ ...filters, layout })}`;
}

export interface AuditExportFilters { userId: string; createdAtStart: string; createdAtEnd: string }

/** Download link for the audit logs matching the filters; empty filters are omitted. */
export function auditExportHref(filters: AuditExportFilters): string {
  return `${AUDIT_EXPORT_PATH}${buildQuery({
    userId: filters.userId.trim() || undefined,
    createdAtStart: filters.createdAtStart || undefined,
    createdAtEnd: filters.createdAtEnd || undefined,
  })}`;
}
