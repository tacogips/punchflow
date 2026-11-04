import { describe, expect, test } from "bun:test";
import { createSignal } from "solid-js";
// The server entry runs Solid components without a DOM, allowing this test to
// initialize the same resource synchronously under Bun.
// @ts-expect-error The package's deep server entry has no declaration file.
import { renderToString } from "solid-js/web/dist/server.js";
import { createApplicationTeamsResource } from "./application-teams";

describe("application team resource initialization", () => {
  test("reads proxy client state while creating the teams resource", () => {
    expect(() => renderToString(() => {
      const [userID] = createSignal("");
      const [proxyClientID] = createSignal("client");
      const [workflowID] = createSignal("workflow");
      createApplicationTeamsResource({ userID, proxyClientID, workflowID });
      return "";
    })).not.toThrow();
  });
});
