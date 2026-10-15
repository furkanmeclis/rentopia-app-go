import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type ContractTemplate = Schemas["ContractTemplate"];
export type ContractTemplatePage =
  Schemas["EnvelopeContractTemplatePage"]["data"];
export type ContractTemplateRequest = Schemas["ContractTemplateRequest"];
export type ContractKind = ContractTemplate["kind"];
export type TemplateStatus = ContractTemplate["status"];
export type ContractPlaceholder = Schemas["ContractPlaceholder"];
export type ContractPlaceholders =
  Schemas["EnvelopeContractPlaceholders"]["data"];
export type ContractPreviewRequest = Schemas["ContractPreviewRequest"];
export type ContractPreview = Schemas["ContractPreview"];

export type ContractDocument = Schemas["Document"];
export type DocumentListItem = Schemas["DocumentListItem"];
export type DocumentPage = Schemas["EnvelopeDocumentPage"]["data"];
export type DocumentVersionInfo = Schemas["DocumentVersionInfo"];
export type DocumentVersion = Schemas["DocumentVersion"];
export type DocumentApproval = Schemas["DocumentApproval"];
export type DocumentPrepareRequest = Schemas["DocumentPrepareRequest"];
export type DocumentPrepareResult = Schemas["EnvelopeDocumentPrepare"]["data"];
export type DocumentApproveRequest = Schemas["DocumentApproveRequest"];
export type DocumentStatus = ContractDocument["status"];
export type VersionStatus = DocumentVersionInfo["status"];

// Remote approval (TEC-711).
export type DocumentSigner = Schemas["DocumentSigner"];
export type DocumentSigners = Schemas["EnvelopeDocumentSigners"]["data"];
export type ApprovalLink = Schemas["DocumentApprovalLink"];
export type ApprovalLinkStatus = ApprovalLink["status"];
export type ApprovalLinkCreateRequest =
  Schemas["DocumentApprovalLinkCreateRequest"];
export type ApprovalLinkCreated =
  Schemas["EnvelopeDocumentApprovalLinkCreated"]["data"];
export type ApprovalLinkSMS = Schemas["DocumentApprovalLinkSMS"];
export type PublicDocument = Schemas["PublicDocument"];
export type PublicDocumentState = PublicDocument["state"];
export type PublicDocumentCode = Schemas["PublicDocumentCode"];

/** List query of `GET /v1/tenant/contract-templates`. */
export type ListTemplatesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  kind?: string;
  status?: string;
};

/** List query of `GET /v1/tenant/documents`. */
export type ListDocumentsParams = {
  limit: number;
  offset: number;
  q?: string;
  kind?: string;
  status?: string;
  reservation?: string;
  rental?: string;
};
