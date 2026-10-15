import { apiConfig } from "@/config/api";
import type {
  ApprovalLink,
  ApprovalLinkCreated,
  ApprovalLinkCreateRequest,
  ApprovalLinkSMS,
  ContractDocument,
  DocumentSigners,
  PublicDocument,
  PublicDocumentCode,
  ContractPlaceholders,
  ContractPreview,
  ContractPreviewRequest,
  ContractTemplate,
  ContractTemplatePage,
  ContractTemplateRequest,
  DocumentApproveRequest,
  DocumentPage,
  DocumentPrepareRequest,
  DocumentPrepareResult,
  DocumentVersion,
  ListDocumentsParams,
  ListTemplatesParams,
} from "@/features/contracts/types";
import type { ResourceMeta } from "@/features/io/types";
import { platformRequest } from "@/lib/api/platform-request";
import { tenantDownloadFile, tenantRequest } from "@/lib/api/tenant-request";

const TEMPLATES = "/v1/tenant/contract-templates";
const DOCUMENTS = "/v1/tenant/documents";
const version = (doc: string, ver: string) =>
  `${DOCUMENTS}/${doc}/versions/${ver}`;

/**
 * Contract templates and documents (`tenantRequest` only). PDFs are
 * streamed by the API (`tenant.document.download`), never from storage.
 */
export const contractsService = {
  templates(slug: string, params: ListTemplatesParams) {
    return tenantRequest<ContractTemplatePage>(slug, "GET", TEMPLATES, {
      query: params,
    });
  },
  templatesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${TEMPLATES}/meta`);
  },
  placeholders(slug: string) {
    return tenantRequest<ContractPlaceholders>(
      slug,
      "GET",
      `${TEMPLATES}/placeholders`,
    );
  },
  template(slug: string, uuid: string) {
    return tenantRequest<ContractTemplate>(slug, "GET", `${TEMPLATES}/${uuid}`);
  },
  createTemplate(slug: string, body: ContractTemplateRequest) {
    return tenantRequest<ContractTemplate>(slug, "POST", TEMPLATES, {
      body,
      silent: true,
    });
  },
  updateTemplate(slug: string, uuid: string, body: ContractTemplateRequest) {
    return tenantRequest<ContractTemplate>(
      slug,
      "PATCH",
      `${TEMPLATES}/${uuid}`,
      { body, silent: true },
    );
  },
  deleteTemplate(slug: string, uuid: string) {
    return tenantRequest<null>(slug, "DELETE", `${TEMPLATES}/${uuid}`);
  },
  activateTemplate(slug: string, uuid: string) {
    return tenantRequest<ContractTemplate>(
      slug,
      "POST",
      `${TEMPLATES}/${uuid}/activate`,
      { body: {} },
    );
  },
  newVersion(slug: string, uuid: string) {
    return tenantRequest<ContractTemplate>(
      slug,
      "POST",
      `${TEMPLATES}/${uuid}/new-version`,
      { body: {} },
    );
  },
  preview(slug: string, body: ContractPreviewRequest) {
    return tenantRequest<ContractPreview>(
      slug,
      "POST",
      `${TEMPLATES}/preview`,
      {
        body,
        silent: true,
      },
    );
  },

  documents(slug: string, params: ListDocumentsParams) {
    return tenantRequest<DocumentPage>(slug, "GET", DOCUMENTS, {
      query: params,
    });
  },
  document(slug: string, uuid: string) {
    return tenantRequest<ContractDocument>(slug, "GET", `${DOCUMENTS}/${uuid}`);
  },
  prepare(slug: string, body: DocumentPrepareRequest) {
    return tenantRequest<DocumentPrepareResult>(slug, "POST", DOCUMENTS, {
      body,
    });
  },
  refresh(slug: string, uuid: string) {
    return tenantRequest<DocumentPrepareResult>(
      slug,
      "POST",
      `${DOCUMENTS}/${uuid}/refresh`,
      { body: {} },
    );
  },
  version(slug: string, doc: string, ver: string) {
    return tenantRequest<DocumentVersion>(slug, "GET", version(doc, ver));
  },
  issue(slug: string, doc: string, ver: string) {
    return tenantRequest<ContractDocument>(
      slug,
      "POST",
      `${version(doc, ver)}/issue`,
      { body: {} },
    );
  },
  approve(
    slug: string,
    doc: string,
    ver: string,
    body: DocumentApproveRequest,
  ) {
    return tenantRequest<ContractDocument>(
      slug,
      "POST",
      `${version(doc, ver)}/approve`,
      { body, silent: true },
    );
  },
  downloadPdf(slug: string, doc: string, ver: string) {
    return tenantDownloadFile(slug, `${version(doc, ver)}/pdf`);
  },

  // Remote approval links (TEC-711).
  signers(slug: string, doc: string) {
    return tenantRequest<DocumentSigners>(
      slug,
      "GET",
      `${DOCUMENTS}/${doc}/signers`,
    );
  },
  approvalLinks(slug: string, doc: string) {
    return tenantRequest<{ items: ApprovalLink[] }>(
      slug,
      "GET",
      `${DOCUMENTS}/${doc}/approval-links`,
    );
  },
  createApprovalLink(
    slug: string,
    doc: string,
    ver: string,
    body: ApprovalLinkCreateRequest,
  ) {
    return tenantRequest<ApprovalLinkCreated>(
      slug,
      "POST",
      `${version(doc, ver)}/approval-links`,
      { body, silent: true },
    );
  },
  sendApprovalLink(slug: string, doc: string, link: string) {
    return tenantRequest<ApprovalLinkSMS>(
      slug,
      "POST",
      `${DOCUMENTS}/${doc}/approval-links/${link}/send-sms`,
      { body: {} },
    );
  },
  revokeApprovalLink(slug: string, doc: string, link: string) {
    return tenantRequest<ApprovalLink>(
      slug,
      "POST",
      `${DOCUMENTS}/${doc}/approval-links/${link}/revoke`,
      { body: {} },
    );
  },
  downloadApprovalPdf(slug: string, doc: string, approval: string) {
    return tenantDownloadFile(
      slug,
      `${DOCUMENTS}/${doc}/approvals/${approval}/pdf`,
    );
  },
};

const PUBLIC_DOCUMENTS = "/v1/public/documents";
const publicPath = (token: string) =>
  `${PUBLIC_DOCUMENTS}/${encodeURIComponent(token)}`;

/**
 * Public remote approval page (`/d/{token}`): no session, the token is the
 * bearer. Errors are rendered by the page (silent).
 */
export const publicDocumentService = {
  get(token: string) {
    return platformRequest<PublicDocument>("GET", publicPath(token), {
      silent: true,
    });
  },
  requestCode(token: string) {
    return platformRequest<PublicDocumentCode>(
      "POST",
      `${publicPath(token)}/code`,
      { body: {}, silent: true },
    );
  },
  approve(token: string, code: string, contentHash: string) {
    return platformRequest<PublicDocument>(
      "POST",
      `${publicPath(token)}/approve`,
      { body: { code, content_hash: contentHash }, silent: true },
    );
  },
  /** Same-origin BFF URL of the approved PDF (streamed by the API). */
  pdfUrl(token: string) {
    return `${apiConfig.baseUrl.replace(/\/$/, "")}${publicPath(token)}/pdf`;
  },
};
