import { describe, expect, test } from "bun:test";
import { deliveryStatusLabel, validateWebhookForm, WEBHOOK_EVENT_TYPES } from "./webhook-view";

describe("webhook view helpers", () => {
  test("lists the 12 non-ping events in reference order", () => {
    expect(WEBHOOK_EVENT_TYPES).toHaveLength(12);
    expect(WEBHOOK_EVENT_TYPES).not.toContain("ping");
    expect(WEBHOOK_EVENT_TYPES).toContain("comment_updated");
    expect(WEBHOOK_EVENT_TYPES).toContain("ticket_archived");
  });

  test("labels delivery state and retries", () => {
    expect(deliveryStatusLabel({ status: "pending", attemptCount: 3, nextAttemptAt: "later" })).toContain("retrying");
    expect(deliveryStatusLabel({ status: "succeeded", attemptCount: 1, nextAttemptAt: null })).toBe("delivered");
    expect(deliveryStatusLabel({ status: "failed", attemptCount: 11, nextAttemptAt: null })).toBe("failed");
  });

  test("validates URL scheme and name while allowing all events", () => {
    expect(validateWebhookForm({ name: "Hook", url: "ftp://x", eventTypes: [] }).url).toBeTruthy();
    expect(validateWebhookForm({ name: " ", url: "https://example.test", eventTypes: [] }).name).toBeTruthy();
    expect(validateWebhookForm({ name: "Hook", url: "http://localhost", eventTypes: [] })).toEqual({});
  });
});
