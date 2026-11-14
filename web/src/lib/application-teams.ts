import { createResource } from "solid-js";
import type { Team } from "../api/types";
import { apiFetch } from "../api/client";

export function createApplicationTeamsResource(readers: {
  userID: () => string;
  proxyClientID: () => string;
  workflowID: () => string;
}) {
  return createResource(applicationTeamsResourceKey(readers), (key) => {
    const [userID, clientID, selectedWorkflowID] = key.split("|");
    if (!userID) return [];
    const query = new URLSearchParams();
    if (clientID) query.set("proxyClientUserId", clientID);
    if (clientID) query.set("workflowId", selectedWorkflowID);
    return apiFetch<Team[]>("GET", `/ui/api/application-teams${query.size ? `?${query}` : ""}`);
  });
}

export function applicationTeamsResourceKey(readers: { userID: () => string; proxyClientID: () => string; workflowID: () => string }): () => string {
  return () => [readers.userID(), readers.proxyClientID(), readers.workflowID()].join("|");
}
