package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"html"
	"io"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
)

const approvalPageCSS = `<style>
.approval-page{break-before:page;page-break-before:always;font-size:10pt}
.approval-page h2{font-size:13pt;margin:0 0 8px}
.approval-page th{width:34%}
.approval-page .mono{font-family:"DejaVu Sans Mono",monospace;word-break:break-all;font-size:8.5pt}
.approval-page .disclaimer{margin-top:14px;border:1px solid #999;padding:8px;background:#f6f6f6}
</style>`

func methodText(method string) string {
	switch method {
	case model.MethodRemoteSMS:
		return "Uzaktan onay: süreli bağlantı ve SMS doğrulama kodu / Remote approval: time-limited link and SMS verification code"
	case model.MethodInPerson:
		return "Yüz yüze (personel tarafından kaydedildi) / In person (recorded by staff)"
	}
	return method
}

// approvalPage is the record appended to the approved document: who
// approved which content (hash), how, when and from where, with the
// required statement that the approval is not a qualified electronic
// signature (TEC-711).
func approvalPage(kind, reference string, a db.GetDocumentApprovalForPDFRow) string {
	row := func(label, value string, mono bool) string {
		class := ""
		if mono {
			class = ` class="mono"`
		}
		return "<tr><th>" + html.EscapeString(label) + "</th><td" + class + ">" + html.EscapeString(value) + "</td></tr>"
	}
	dash := func(v string) string {
		if strings.TrimSpace(v) == "" {
			return "—"
		}
		return v
	}
	ip := ""
	if a.IpAddress != nil {
		ip = a.IpAddress.String()
	}
	var b strings.Builder
	b.WriteString(approvalPageCSS)
	b.WriteString(`<section class="approval-page"><h2>Onay Kaydı / Approval Record</h2><table><tbody>`)
	b.WriteString(row("Belge / Document", kindTitles[kind]+" · "+reference+" · Sürüm / Version "+strconv.Itoa(int(a.VersionNumber)), false))
	b.WriteString(row("İçerik özeti (SHA-256) / Content hash", a.ContentHash, true))
	b.WriteString(row("Onaylayan / Approved by", a.ApproverName, false))
	b.WriteString(row("Yöntem / Method", methodText(a.Method), false))
	if a.Method == model.MethodRemoteSMS {
		b.WriteString(row("Doğrulanan telefon / Verified phone", dash(a.VerifiedPhoneMasked), false))
	}
	b.WriteString(row("Onay zamanı / Approved at", rentaltime.Local(a.ApprovedAt.Time).Format("02.01.2006 15:04:05")+" (Europe/Istanbul)", false))
	if a.Method == model.MethodRemoteSMS {
		b.WriteString(row("IP adresi / IP address", dash(ip), false))
		b.WriteString(row("Cihaz / Device (user agent)", dash(a.UserAgent), true))
	}
	b.WriteString(row("Onay no / Approval id", a.Uuid.String(), true))
	if a.InvalidatedAt.Valid {
		b.WriteString(row("Durum / Status", "Belgenin yeni sürümü bu onayı geçersiz kıldı / A newer version invalidated this approval", false))
	}
	b.WriteString(`</tbody></table><p class="disclaimer"><strong>`)
	b.WriteString(html.EscapeString(model.DisclaimerTR))
	b.WriteString(`</strong><br>`)
	b.WriteString(html.EscapeString(model.DisclaimerEN))
	b.WriteString(`</p></section>`)
	return b.String()
}

// withApprovalPage inserts the approval page at the end of the document.
func withApprovalPage(doc, page string) string {
	if i := strings.LastIndex(strings.ToLower(doc), "</body>"); i >= 0 {
		return doc[:i] + page + doc[i:]
	}
	return doc + page
}

// approvalPDF streams the approval PDF (rendered and stored privately on
// first use, under the document's prefix).
func (s *Service) approvalPDF(ctx context.Context, orgID int64, docUUID uuid.UUID, kind, reference string, a db.GetDocumentApprovalForPDFRow) (PDFFile, error) {
	if s.storage == nil {
		return PDFFile{}, ErrStorageUnavailable
	}
	org, err := s.q.GetDocumentOrganization(ctx, orgID)
	if err != nil {
		return PDFFile{}, err
	}
	name := strings.ToLower(reference) + "-" + kind + "-v" + strconv.Itoa(int(a.VersionNumber)) + "-onay.pdf"
	if a.PdfObjectKey.Valid && storage.OrganizationOwnsKey(a.PdfObjectKey.String, org.Uuid.String()) {
		rc, size, err := s.storage.Download(ctx, a.PdfObjectKey.String)
		if err == nil {
			if size <= 0 {
				size = a.PdfSizeBytes
			}
			return PDFFile{Body: rc, Size: size, Filename: name, Version: a.VersionNumber, Kind: kind}, nil
		}
		s.log.Warn("approval_pdf_download_failed", "approval", a.Uuid.String(), "error", err)
	}
	if s.renderer == nil {
		return PDFFile{}, ErrPDFNotReady
	}
	pdf, err := s.renderer.RenderPDF(ctx, []byte(withApprovalPage(a.Html, approvalPage(kind, reference, a))))
	if err != nil {
		s.log.Warn("approval_pdf_render_failed", "approval", a.Uuid.String(), "error", err)
		return PDFFile{}, ErrPDFNotReady
	}
	key := storage.DocumentPDFObjectKey(org.Uuid.String(), docUUID.String(), a.VersionUuid.String()+"-approval-"+a.Uuid.String())
	if err := s.storage.Upload(ctx, storage.File{
		Body: bytes.NewReader(pdf), Size: int64(len(pdf)), ContentType: "application/pdf", Filename: name,
	}, key); err != nil {
		s.log.Warn("approval_pdf_store_failed", "approval", a.Uuid.String(), "error", err)
	} else {
		sum := sha256.Sum256(pdf)
		_ = s.q.SetDocumentApprovalPDF(ctx, db.SetDocumentApprovalPDFParams{
			OrganizationID: orgID, ID: a.ID, PdfObjectKey: textOf(key), PdfSizeBytes: int64(len(pdf)),
			PdfSha256: textOf(hex.EncodeToString(sum[:])),
		})
	}
	return PDFFile{Body: io.NopCloser(bytes.NewReader(pdf)), Size: int64(len(pdf)), Filename: name, Version: a.VersionNumber, Kind: kind}, nil
}
