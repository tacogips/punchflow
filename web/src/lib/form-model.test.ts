import { describe, expect, test } from "bun:test";
import { initialFormState, toTicketInputs } from "./form-model";
import type { WorkflowDetail } from "../api/types";

const workflow: WorkflowDetail = {
  id: "workflow", name: "Example", sectionList: [{
    id: "form", sectionType: "form", title: "Inputs", formFields: [
      { id: "1", code: "text", name: "Text", fieldType: "text" },
      { id: "2", code: "number", name: "Number", fieldType: "number" },
      { id: "3", code: "check", name: "Check", fieldType: "checkbox" },
      { id: "4", code: "master", name: "Master", fieldType: "master" },
      { id: "5", code: "file", name: "File", fieldType: "file" },
      { id: "6", code: "calc", name: "Calculation", fieldType: "calculation" },
    ],
  }],
};

describe("form model", () => {
  test("creates one API input per form field with field-specific values", () => {
    const state = initialFormState(workflow);
    state.text.value = "hello";
    state.number.value = 12;
    state.check.value = ["a", "b"];
    state.master.value = "master-item";
    state.file.value = "signed-file";
    const inputs = toTicketInputs(workflow, state);
    expect(inputs).toEqual([
      { formFieldCode: "text", value: "hello" }, { formFieldCode: "number", value: 12 },
      { formFieldCode: "check", value: ["a", "b"] }, { formFieldCode: "master", generalMasterItemId: "master-item" },
      { formFieldCode: "file", files: ["signed-file"] }, { formFieldCode: "calc", value: null },
    ]);
  });
});
