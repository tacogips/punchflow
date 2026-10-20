import { For, Show, createResource, lazy } from "solid-js";
import type { JSX } from "solid-js";
import { A, Route, Router, Navigate } from "@solidjs/router";
import { loadSession, session } from "./session";

const Login = lazy(() => import("./pages/Login"));
const Dashboard = lazy(() => import("./pages/Dashboard"));
const TicketSearch = lazy(() => import("./pages/tickets/TicketSearch"));
const TicketDetail = lazy(() => import("./pages/tickets/TicketDetail"));
const TicketEdit = lazy(() => import("./pages/tickets/TicketEdit"));
const Workflows = lazy(() => import("./pages/definitions/Workflows"));
const WorkflowView = lazy(() => import("./pages/definitions/WorkflowView"));
const RouteList = lazy(() => import("./pages/definitions/Routes"));
const RouteView = lazy(() => import("./pages/definitions/RouteView"));
const Users = lazy(() => import("./pages/admin/Users"));
const Org = lazy(() => import("./pages/admin/Org"));
const Grades = lazy(() => import("./pages/admin/Grades"));
const Roles = lazy(() => import("./pages/admin/Roles"));
const Masters = lazy(() => import("./pages/admin/Masters"));
const Classification = lazy(() => import("./pages/admin/Classification"));
const Proxies = lazy(() => import("./pages/admin/Proxies"));
const AuditLogs = lazy(() => import("./pages/admin/AuditLogs"));
const Tokens = lazy(() => import("./pages/settings/Tokens"));
const ServiceAccounts = lazy(() => import("./pages/admin/ServiceAccounts"));
const Webhooks = lazy(() => import("./pages/admin/Webhooks"));
const WebhookEvents = lazy(() => import("./pages/admin/WebhookEvents"));
const DevTools = lazy(() => import("./pages/admin/DevTools"));

const navigation = [
  ["Dashboard", "/"], ["Tickets", "/tickets"], ["New ticket", "/tickets/new"],
  ["Workflows", "/workflows"], ["Routes", "/routes"], ["Users", "/admin/users"],
  ["Organization", "/admin/org"], ["Grades", "/admin/grades"], ["Roles", "/admin/roles"],
  ["Masters", "/admin/masters"], ["Classification", "/admin/classification"],
  ["Proxies", "/admin/proxies"], ["Audit logs", "/admin/audit-logs"],
  ["API tokens", "/settings/tokens"], ["Service accounts", "/admin/service-accounts"],
  ["Webhooks", "/admin/webhooks"], ["Webhook events", "/admin/webhooks/example/events"], ["Dev tools", "/admin/dev"],
] as const;

function Layout(props: { children?: JSX.Element }) {
  const [current] = createResource(loadSession);
  return <Show when={!current.loading} fallback={<main class="loading">Loading session…</main>}>
    <Show when={session()} fallback={<Navigate href="/login" />}>
      <div class="app-shell">
        <aside class="sidebar"><A class="brand" href="/">Punchflow</A><nav aria-label="Main navigation">
          <For each={navigation}>{([label, href]) => <A href={href} end={href === "/"}>{label}</A>}</For>
        </nav><div class="user-label">{session()?.user.name}</div></aside>
        <main class="content">{props.children}</main>
      </div>
    </Show>
  </Show>;
}

function Protected(props: { children?: JSX.Element }) { return <Layout>{props.children}</Layout>; }

export default function App() {
  return <Router>
    <Route path="/login" component={Login} />
    <Route path="/" component={() => <Protected><Dashboard /></Protected>} />
    <Route path="/tickets" component={() => <Protected><TicketSearch /></Protected>} />
    <Route path="/tickets/new" component={() => <Protected><TicketEdit /></Protected>} />
    <Route path="/tickets/:id" component={() => <Protected><TicketDetail /></Protected>} />
    <Route path="/tickets/:id/edit" component={() => <Protected><TicketEdit /></Protected>} />
    <Route path="/workflows" component={() => <Protected><Workflows /></Protected>} />
    <Route path="/workflows/:id" component={() => <Protected><WorkflowView /></Protected>} />
    <Route path="/routes" component={() => <Protected><RouteList /></Protected>} />
    <Route path="/routes/:id" component={() => <Protected><RouteView /></Protected>} />
    <Route path="/admin/users" component={() => <Protected><Users /></Protected>} />
    <Route path="/admin/org" component={() => <Protected><Org /></Protected>} />
    <Route path="/admin/grades" component={() => <Protected><Grades /></Protected>} />
    <Route path="/admin/roles" component={() => <Protected><Roles /></Protected>} />
    <Route path="/admin/masters" component={() => <Protected><Masters /></Protected>} />
    <Route path="/admin/classification" component={() => <Protected><Classification /></Protected>} />
    <Route path="/admin/proxies" component={() => <Protected><Proxies /></Protected>} />
    <Route path="/admin/audit-logs" component={() => <Protected><AuditLogs /></Protected>} />
    <Route path="/settings/tokens" component={() => <Protected><Tokens /></Protected>} />
    <Route path="/admin/service-accounts" component={() => <Protected><ServiceAccounts /></Protected>} />
    <Route path="/admin/webhooks" component={() => <Protected><Webhooks /></Protected>} />
    <Route path="/admin/webhooks/:id/events" component={() => <Protected><WebhookEvents /></Protected>} />
    <Route path="/admin/dev" component={() => <Protected><DevTools /></Protected>} />
    <Route path="*" component={() => <Navigate href="/" />} />
  </Router>;
}
