package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
)

func (d *Domains) Code() string { return "custom_domains" }

// Collect exposes failed verification and TLS work through the existing platform alerts.
func (d *Domains) Collect(ctx context.Context, _ time.Time) (healthcheck.Report, error) {
	rows, err := d.q.ListSiteDomainHealthIssues(ctx, 501)
	if err != nil {
		return healthcheck.Report{}, err
	}
	report := healthcheck.Report{Issues: []healthcheck.Issue{}, Metrics: []healthcheck.Metric{}, Truncated: len(rows) > 500}
	if len(rows) > 500 {
		rows = rows[:500]
	}
	counts, err := d.q.SiteDomainHealthCounts(ctx)
	if err != nil {
		return healthcheck.Report{}, err
	}
	for _, r := range rows {
		if r.Status != "failed" && r.TlsStatus != "failed" && r.TlsStatus != "manual" {
			continue
		}
		severity, message := healthcheck.SeverityCritical, r.LastError
		if r.TlsStatus == "manual" && r.Status == "verified" {
			severity, message = healthcheck.SeverityWarning, d.view(r).TLSInstructions
		}
		orgID := r.OrganizationID
		occurred := r.UpdatedAt.Time
		if r.AttentionSince.Valid {
			occurred = r.AttentionSince.Time
		}
		report.Issues = append(report.Issues, healthcheck.Issue{Kind: "domain_attention", Severity: severity, OrganizationID: &orgID,
			Provider: r.Provisioner, Subject: r.Host, Message: message, RefType: "site_domain", RefUUID: r.Uuid.String(), OccurredAt: occurred})
	}
	report.Metrics = []healthcheck.Metric{{Key: "verified", Value: counts.Verified}, {Key: "pending", Value: counts.Pending}, {Key: "failed", Value: counts.Failed, Severity: healthcheck.SeverityCritical}}
	return report, nil
}
