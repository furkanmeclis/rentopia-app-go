package storage

import "fmt"

// AppLogoObjectKey builds app/branding/logo.{ext} for system letterhead.
func AppLogoObjectKey(ext string) string {
	return fmt.Sprintf("app/branding/logo.%s", trimExt(ext))
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

func trimExt(ext string) string {
	for len(ext) > 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	return ext
}
