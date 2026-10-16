package storage

import (
	"fmt"
	"strings"
)

// AppLogoObjectKey builds app/branding/logo.{ext} for system letterhead.
func AppLogoObjectKey(ext string) string {
	return fmt.Sprintf("app/branding/logo.%s", trimExt(ext))
}

// OrganizationLogoObjectKey builds
// organizations/{orgUUID}/branding/logo-{versionUUID}.{ext}. Every upload
// gets a new version so the API logo URL changes with it (cache busting).
func OrganizationLogoObjectKey(orgUUID, versionUUID, ext string) string {
	return fmt.Sprintf("organizations/%s/branding/logo-%s.%s", orgUUID, versionUUID, trimExt(ext))
}

// ExportObjectKey builds exports/{jobUUID}/export.{ext}.
func ExportObjectKey(jobUUID, ext string) string {
	return fmt.Sprintf("exports/%s/export.%s", jobUUID, trimExt(ext))
}

// OrganizationsRoot is the common prefix of organization-owned objects.
const OrganizationsRoot = "organizations/"

// OrganizationPrefix is the key prefix of every object an organization owns:
// organizations/{orgUUID}/. Tenant uploads and engine files live under it, so
// an organization's objects can be listed, checked and purged together.
func OrganizationPrefix(orgUUID string) string {
	return OrganizationsRoot + orgUUID + "/"
}

// OrganizationOwnsKey reports whether key lies under the organization's
// prefix. Streaming endpoints check it before serving a key read from a row
// (defence in depth on top of the organization filter of the query).
func OrganizationOwnsKey(key, orgUUID string) bool {
	return orgUUID != "" && strings.HasPrefix(key, OrganizationPrefix(orgUUID)) && !strings.Contains(key, "/../")
}

// OrganizationExportObjectKey builds
// organizations/{orgUUID}/exports/{jobUUID}/export.{ext} for tenant exports.
func OrganizationExportObjectKey(orgUUID, jobUUID, ext string) string {
	return fmt.Sprintf("%sexports/%s/export.%s", OrganizationPrefix(orgUUID), jobUUID, trimExt(ext))
}

// OrganizationImportSourceObjectKey builds
// organizations/{orgUUID}/imports/{jobUUID}/source.{ext} for tenant imports.
func OrganizationImportSourceObjectKey(orgUUID, jobUUID, ext string) string {
	return fmt.Sprintf("%simports/%s/source.%s", OrganizationPrefix(orgUUID), jobUUID, trimExt(ext))
}

// ImportSourceObjectKey builds imports/{jobUUID}/source.{ext}.
func ImportSourceObjectKey(jobUUID, ext string) string {
	return fmt.Sprintf("imports/%s/source.%s", jobUUID, trimExt(ext))
}

// CatalogImportObjectKey builds catalog/imports/{importUUID}/source.{ext}
// for the private TSB kasko list uploads.
func CatalogImportObjectKey(importUUID, ext string) string {
	return fmt.Sprintf("catalog/imports/%s/source.%s", importUUID, trimExt(ext))
}

// CatalogBrandLogoObjectKey builds catalog/brands/{displayBrandUUID}/logo.{ext}
// for catalog display brand logos (served through the API, never by URL).
func CatalogBrandLogoObjectKey(displayBrandUUID, ext string) string {
	return fmt.Sprintf("catalog/brands/%s/logo.%s", displayBrandUUID, trimExt(ext))
}

// RentalProductImageObjectKey builds
// organizations/{orgUUID}/rental-products/{productUUID}/images/{imageUUID}.{ext}
// for rental product images (served through the API's public media route).
func RentalProductImageObjectKey(orgUUID, productUUID, imageUUID, ext string) string {
	return fmt.Sprintf("organizations/%s/rental-products/%s/images/%s.%s", orgUUID, productUUID, imageUUID, trimExt(ext))
}

// VehicleDocumentObjectKey builds
// organizations/{orgUUID}/vehicles/{vehicleUUID}/documents/{documentUUID}/{fileUUID}.{ext}
// for scanned vehicle documents (private: streamed by the API only, never
// linked from the browser). A replaced file gets a new fileUUID.
func VehicleDocumentObjectKey(orgUUID, vehicleUUID, documentUUID, fileUUID, ext string) string {
	return fmt.Sprintf("organizations/%s/vehicles/%s/documents/%s/%s.%s", orgUUID, vehicleUUID, documentUUID, fileUUID, trimExt(ext))
}

// CustomerDocumentObjectKey builds
// organizations/{orgUUID}/customers/{customerUUID}/documents/{documentUUID}/{fileUUID}.{ext}
// for identity / licence scans and other customer documents (private:
// streamed by the API only after the reveal permission check). A replaced
// file gets a new fileUUID.
func CustomerDocumentObjectKey(orgUUID, customerUUID, documentUUID, fileUUID, ext string) string {
	return fmt.Sprintf("organizations/%s/customers/%s/documents/%s/%s.%s", orgUUID, customerUUID, documentUUID, fileUUID, trimExt(ext))
}

// DocumentPDFObjectKey builds
// organizations/{orgUUID}/documents/{documentUUID}/{versionUUID}.pdf for the
// rendered contract / form PDFs (private: streamed by the API only, behind
// tenant.document.download). A version is immutable, so is its PDF.
func DocumentPDFObjectKey(orgUUID, documentUUID, versionUUID string) string {
	return fmt.Sprintf("%sdocuments/%s/%s.pdf", OrganizationPrefix(orgUUID), documentUUID, versionUUID)
}

// InspectionPhotoObjectKey builds
// organizations/{orgUUID}/rentals/{rentalUUID}/inspections/{inspectionUUID}/photos/{photoUUID}.{ext}
// for handover / return evidence photos (private: streamed by the API
// only, through short-lived media links).
func InspectionPhotoObjectKey(orgUUID, rentalUUID, inspectionUUID, photoUUID, ext string) string {
	return fmt.Sprintf("%srentals/%s/inspections/%s/photos/%s.%s", OrganizationPrefix(orgUUID), rentalUUID, inspectionUUID, photoUUID, trimExt(ext))
}

// InspectionSignatureObjectKey builds
// organizations/{orgUUID}/rentals/{rentalUUID}/inspections/{inspectionUUID}/signatures/{signatureUUID}.png
// for the signature canvas PNG of a handover / return form (private).
func InspectionSignatureObjectKey(orgUUID, rentalUUID, inspectionUUID, signatureUUID string) string {
	return fmt.Sprintf("%srentals/%s/inspections/%s/signatures/%s.png", OrganizationPrefix(orgUUID), rentalUUID, inspectionUUID, signatureUUID)
}

// ExpenseAttachmentObjectKey builds
// organizations/{orgUUID}/expenses/{expenseUUID}/{fileUUID}.{ext} for the
// receipt / invoice of an expense (private: streamed by the API only). A
// replaced file gets a new fileUUID.
func ExpenseAttachmentObjectKey(orgUUID, expenseUUID, fileUUID, ext string) string {
	return fmt.Sprintf("%sexpenses/%s/%s.%s", OrganizationPrefix(orgUUID), expenseUUID, fileUUID, trimExt(ext))
}

// SupplierInvoiceAttachmentObjectKey builds
// organizations/{orgUUID}/supplier-invoices/{invoiceUUID}/{fileUUID}.{ext}
// for the scanned / PDF supplier invoice (private: streamed by the API
// only). A replaced file gets a new fileUUID.
func SupplierInvoiceAttachmentObjectKey(orgUUID, invoiceUUID, fileUUID, ext string) string {
	return fmt.Sprintf("%ssupplier-invoices/%s/%s.%s", OrganizationPrefix(orgUUID), invoiceUUID, fileUUID, trimExt(ext))
}

func trimExt(ext string) string {
	for len(ext) > 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	return ext
}
