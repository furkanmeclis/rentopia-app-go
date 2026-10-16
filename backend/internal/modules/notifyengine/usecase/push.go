package usecase

import (
	"context"
	"net/url"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/model"
)

// SubscribeCustomerPush stores a customer's browser push subscription
// (customer portal). Customer subscriptions never mix with members'.
func (e *Engine) SubscribeCustomerPush(ctx context.Context, orgID, customerID int64, in model.PushSubscriptionInput) error {
	ve := &ValidationError{}
	endpoint := strings.TrimSpace(in.Endpoint)
	if u, err := url.Parse(endpoint); err != nil || u.Scheme != "https" || u.Host == "" || len(endpoint) > 2048 {
		ve.add("endpoint", "invalid", "a https push endpoint is required")
	}
	if strings.TrimSpace(in.KeyP256dh) == "" || len(in.KeyP256dh) > 256 {
		ve.add("key_p256dh", "required", "the subscription key is required")
	}
	if strings.TrimSpace(in.KeyAuth) == "" || len(in.KeyAuth) > 128 {
		ve.add("key_auth", "required", "the subscription secret is required")
	}
	if err := ve.orNil(); err != nil {
		return err
	}
	_, err := e.q.UpsertCustomerPushSubscription(ctx, db.UpsertCustomerPushSubscriptionParams{
		OrganizationID: orgID, CustomerID: customerID, Endpoint: endpoint,
		KeyP256dh: strings.TrimSpace(in.KeyP256dh), KeyAuth: strings.TrimSpace(in.KeyAuth),
	})
	return err
}

// UnsubscribeCustomerPush removes a customer's subscription.
func (e *Engine) UnsubscribeCustomerPush(ctx context.Context, orgID, customerID int64, endpoint string) error {
	_, err := e.q.DeleteCustomerPushSubscription(ctx, db.DeleteCustomerPushSubscriptionParams{
		OrganizationID: orgID, CustomerID: customerID, Endpoint: strings.TrimSpace(endpoint),
	})
	return err
}
