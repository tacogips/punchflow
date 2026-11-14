import { describe, expect, test } from "bun:test";
import { parseLinkHeader } from "./link";

describe("parseLinkHeader", () => {
  test("parses relation links and empty headers", () => {
    expect(parseLinkHeader('<http://x/v1/users?page=2&perPage=25>; rel="next", <http://x/v1/users?page=5&perPage=25>; rel="last"')).toEqual({ next: "http://x/v1/users?page=2&perPage=25", last: "http://x/v1/users?page=5&perPage=25" });
    expect(parseLinkHeader(null)).toEqual({});
  });
});
