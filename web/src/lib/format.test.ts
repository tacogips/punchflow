import { describe, expect, test } from "bun:test";
import { formatJst } from "./format";

describe("formatJst", () => {
  test("formats JST date and minute", () => {
    expect(formatJst("2020-05-01T12:34:56.789+09:00")).toBe("2020-05-01 12:34");
    expect(formatJst(null)).toBe("");
  });
});
