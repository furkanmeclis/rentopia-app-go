package storage

import "fmt"

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

func trimExt(ext string) string {
	for len(ext) > 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	return ext
}
