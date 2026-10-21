import { describe, expect, test } from "bun:test";
import type { FormField, GeneralMasterItem } from "../api/types";
import { fieldInputAllowed, filterMasterItems, ticketCopyCompatible, ticketCopyValue } from "./form-ui";

const field = (code: string, fieldType = "text", extra: Record<string, unknown> = {}): FormField => ({ id: code, code, name: code, fieldType, ...extra });

describe("form UI metadata", () => {
  test("hidden, read-only and actor-only fields are not editable", () => {
    expect(fieldInputAllowed(field("hidden", "text", { hidden: true, author: true }), "applicant")).toBe(false);
    expect(fieldInputAllowed(field("readonly", "text", { readonlyOnUi: true, author: true }), "applicant")).toBe(false);
    expect(fieldInputAllowed(field("approver", "text", { author: false, approver: true }), "applicant")).toBe(false);
    expect(fieldInputAllowed(field("author", "text", { author: true, approver: false }), "approver")).toBe(false);
    expect(fieldInputAllowed(field("editable", "text", { author: true }), "applicant")).toBe(true);
  });

  test("automatic master filters use the configured source field and master property", () => {
    const source = field("region", "text", { id: "region-id" });
    const picker = field("office", "general_master", { generalMasterSearchFilters: [{ id: "filter", filterFormFieldId: "region-id", fieldType: "custom_field", generalMasterFieldId: "region-field" }] });
    const items = [
      { id: "tokyo", code: "tko", name: "Tokyo", inputs: [{ value: "East", field: { id: "region-field" } }] },
      { id: "osaka", code: "osk", name: "Osaka", inputs: [{ value: "West", field: { id: "region-field" } }] },
    ] as unknown as GeneralMasterItem[];
    expect(filterMasterItems(items, picker, { region: { value: "East" } }, [source]).map((item) => item.id)).toEqual(["tokyo"]);
    expect(filterMasterItems(items, picker, { region: { value: "" } }, [source])).toEqual(items);
    expect(filterMasterItems(items, picker, { region: { value: "West" } }, [source]).map((item) => item.id)).toEqual(["osaka"]);
  });

  test("ticket copy checks compatible type and destination actor permissions", () => {
    const target = field("summary", "text", { author: true });
    expect(ticketCopyCompatible("ticket_title", target)).toBe(true);
    expect(ticketCopyValue({ fieldType: "ticket_title", fieldCode: null, targetFieldCode: "summary" }, { title: "Copied" }, target, "applicant")).toBe("Copied");
    expect(ticketCopyValue({ fieldType: "form_field", fieldCode: "source", targetFieldCode: "summary" }, { inputs: [{ formFieldCode: "source", value: "Copied from source", formField: { fieldType: "text" } }] }, target, "applicant")).toBe("Copied from source");
    expect(ticketCopyValue({ fieldType: "form_field", fieldCode: "total", targetFieldCode: "summary" }, { inputs: [{ formFieldCode: "total", value: 123.5, formField: { fieldType: "calculation" } }] }, target, "applicant")).toBe("123.5");
    expect(ticketCopyValue({ fieldType: "ticket_title", fieldCode: null, targetFieldCode: "summary" }, { title: "Copied" }, field("locked", "text", { author: false }), "applicant")).toBeUndefined();
    expect(ticketCopyCompatible("date", field("date-time", "datetime"))).toBe(false);
    expect(ticketCopyValue({ fieldType: "form_field", fieldCode: "choice", targetFieldCode: "target" }, { inputs: [{ formFieldCode: "choice", value: "Other", formField: { fieldType: "pull_down" } }] }, field("target", "pull_down", { author: true, options: ["Allowed"] }), "applicant")).toBeUndefined();
  });
});
