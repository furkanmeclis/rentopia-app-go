# Notification engine (TEC-733)

Organization-scoped customer and staff notifications: an event is routed to
an ordered list of channels with fallback, rendered from a template and
logged per channel attempt. Module: `backend/internal/modules/notifyengine`.
UI: `/t/{slug}/settings/notifications` (rules, templates, delivery history).

## Model

| Table | Purpose |
|-------|---------|
| `notification_rules` | Organization override of an event's rule: `enabled`, ordered `channels`, `fallback`, `reminder_offsets` (minutes before the deadline). No row = catalog default. |
| `notification_message_templates` | Organization template per event / channel / language (`tr`, `en`). No row = catalog default. |
| `notification_dispatches` | One event for one recipient (customer or member). `idempotency_key` (event + reference + discriminator) is unique per organization. Holds the safe message variables, the channel snapshot and the next channel index. |
| `notification_deliveries` | One row per dispatch and channel (unique): provider, provider reference, masked recipient, rendered text, result, error. |
| `customer_push_subscriptions` | Customer portal web push subscriptions, apart from members' `push_subscriptions`. |

Permissions: `tenant.notification_settings.read` (rules, templates, history)
and `.write` (edit, reset, test send).

## Events

The catalog (`notifyengine/catalog`) defines audience, default rule,
variables and tr / en templates:

- reservations: `reservation.confirmed`, `reservation.rejected`,
  `reservation.cancelled`, `reservation.payment_pending` (instant +
  reminder before the payment deadline);
- rentals: `rental.pickup_upcoming`, `rental.return_upcoming` (reminders),
  `rental.overdue`;
- finance: `invoice.issued`, `account.payment_due` (reminder before the
  due date, 09:00 Istanbul);
- operations (members): `field_task.assigned`, `field_task.overdue`.

Sources:

- reservation transitions call `Engine.ReservationChanged` inside the
  transition's transaction (reservations `TransitionNotifier` port), so the
  dispatch commits with the status change;
- the worker's minute sweep (`app:notify-engine:sweep`) creates reminder
  and sweep-detected dispatches (pickup / return / payment deadlines,
  overdue rentals, issued invoices, account due dates, field task
  assignments and overdue tasks) and sends due dispatches. Reminder keys
  are `remind:<offset>:<deadline>`; of missed offsets only the smallest due
  one is sent.

## Delivery

`Engine.Process` claims the dispatch (row lock), inserts the attempt row of
the next channel (unique dispatch + channel: a retry never sends the same
event twice on one channel), renders the template and calls the channel.

| Channel result | Attempt | Dispatch |
|----------------|---------|----------|
| delivered / sent | delivered / sent | delivered |
| accepted (async) | sent, waits for the report | sending |
| failed / skipped | failed / skipped | next channel when `fallback`, else failed (skipped when every attempt was skipped) |

A delivery report (`Engine.ReportDelivery`) settles an accepted attempt; a
failed report runs the next channel (RT-A30).

## Channels

`notifyengine/channel.Channel` is the adapter contract (`Name`, `Supports`,
`Send`); `ReportingChannel` adds delivery reports. Adapters:

- `sms`: the tenant SMS module (TEC-734), class transactional, never
  marketing consent; the SMS log keeps the text with the link redacted. The
  adapter listens to the integration runner and reports the final state of
  the send job (found through `sms_messages.ref_uuid` = delivery uuid).
- `email`: the SMTP sender.
- `push`: members through the notification center (inbox, web push, mobile
  push); customers through `customer_push_subscriptions` (VAPID).
- `whatsapp`: the organization's linked WhatsApp session (TEC-735,
  [whatsapp.md](./whatsapp.md)), registered with `Engine.Register`; skipped
  while not linked, delivery receipts settle the attempt.

## Privacy

Templates may only use their event's variables (names, references, dates,
branch names, a portal link that asks for a one-time code). Messages never
carry identity, licence, signature or detailed financial data. Marketing
communication preferences (İYS consents) stay separate: these messages are
transactional.
