export interface User { id: string; name: string; email: string; status?: string; [key: string]: unknown }
export interface Team { id: string; name: string; fullName?: string; code?: string | null; [key: string]: unknown }
export interface Grade { id: string; name: string; code?: string; [key: string]: unknown }
export interface Role { id: string; name: string; permissionList?: string[]; [key: string]: unknown }
export interface Folder { id: string; name: string; [key: string]: unknown }
export interface Category { id: string; name: string; [key: string]: unknown }
export interface GeneralMasterItem { id: string; code: string; name: string; [key: string]: unknown }
export interface GeneralMaster { id: string; code: string; name: string; items?: GeneralMasterItem[]; [key: string]: unknown }
export interface FormField { id: string; code: string; name: string; fieldType: string; [key: string]: unknown }
export interface SlipField extends FormField {}
export interface SectionListItem {
  id: string; sectionType: "form" | "slip"; title: string | null;
  formFields?: FormField[]; slipFields?: SlipField[]; code?: string;
}
export interface Workflow { id: string; name: string; [key: string]: unknown }
export interface WorkflowDetail extends Workflow { sectionList: SectionListItem[] }
export interface RouteStep { id: string; title: string; stepOrder?: number; [key: string]: unknown }
export interface Route { id: string; name: string; [key: string]: unknown }
export interface RouteDetail extends Route { steps: RouteStep[] }
export interface TicketAssignee { user?: User | null; [key: string]: unknown }
export interface TicketStep { id: string; title: string; assignees?: TicketAssignee[]; [key: string]: unknown }
export interface TicketInput { formFieldCode?: string; formFieldId?: string; value?: unknown; [key: string]: unknown }
export interface TicketInputBody {
  formFieldCode?: string;
  value?: unknown;
  generalMasterItemId?: unknown;
  userId?: unknown;
  teamId?: unknown;
  ticketId?: unknown;
  files?: string[] | null;
}
export interface SlipItemBody { slipSectionId?: string; slipSectionCode?: string; inputs: Array<TicketInputBody & { slipFieldCode: string }> }
export interface TicketWithStep { id: string; ticketNumber: string | null; title: string | null; status: string; steps: TicketStep[]; [key: string]: unknown }
export interface TicketDetail extends TicketWithStep { inputs: TicketInput[]; route: RouteDetail | null; [key: string]: unknown }
export interface Comment { id: string; content: string; [key: string]: unknown }
export interface TicketViewer { id: string; [key: string]: unknown }
export interface AuditLog { id: string; action: string; createdAt: string; [key: string]: unknown }
export interface ErrorBody { code: string; message: string; errors?: Record<string, string[]>; doc_url?: string }
export interface ApiToken { id: string; name: string; createdAt: string; lastUsedAt?: string | null }
export interface ServiceAccount { id: string; name: string; description?: string }
export interface Webhook { id: string; name: string; url: string; enabled: boolean; eventTypes: string[] }
export interface WebhookAttempt { id: string; statusCode?: number | null; createdAt: string; [key: string]: unknown }
export interface WebhookEvent { id: string; eventType: string; attempts: WebhookAttempt[]; [key: string]: unknown }
export interface TenantSettings { name: string; plan: string; mode: string; [key: string]: unknown }
export interface FaultRule { id: string; pathPrefix: string; status: number; code: string; [key: string]: unknown }
export interface Label { id: string; name: string; description: string | null; color: string; createdAt: string; updatedAt: string }
export interface SubStatus {
  id: string; code: string; name: string; notes: string | null;
  allowWithdrawal: boolean; allowRejection: boolean; allowDenial: boolean; createdAt: string; updatedAt: string;
}
/** Runtime lifecycle controls the caller may use on a ticket (GET /ui/api/tickets/{id}/controls). */
export interface TicketControls {
  cancelApproval: boolean;
  raise: boolean;
  raiseTargetStepOrders: number[];
  addCustomStep: boolean;
  customStepInsertAfter: number[];
  removableCustomStepIds: string[];
}
export interface CustomStepBody {
  title: string;
  actionType: "approve" | "confirm";
  userIds: string[];
  instruction?: string;
  afterStepOrder?: number;
}
