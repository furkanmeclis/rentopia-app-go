package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/cache"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/logging"
	agreementadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/adapters"
	agreementusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/usecase"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/adapters"
	branchrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	bulkusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/usecase"
	campaignsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/adapters"
	campaignsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	catalogrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	catalogusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/usecase"
	channelsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels"
	channeladapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/adapters"
	channelusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/usecase"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	customeradapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	customerrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	customerusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	exportusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/exports/usecase"
	extrasadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/adapters"
	extrasusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/usecase"
	fieldtasksadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/adapters"
	fieldtasksusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/usecase"
	financeadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/adapters"
	financeusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	fleetadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/adapters"
	fleetrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/repository"
	fleetusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/usecase"
	fleetopsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/adapters"
	fleetopsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	fleetopsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	importusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/imports/usecase"
	integrationaccountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	integrationjobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	invoicesmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices"
	invoiceadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/adapters"
	invoiceusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/usecase"
	kabismodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis"
	kabisusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/usecase"
	logsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/logs/usecase"
	notifmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/usecase"
	notifyengineadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/adapters"
	notifyengineusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/usecase"
	orgadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/adapters"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	platformhealthmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth"
	platformhealthusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/usecase"
	pricingadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/adapters"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	profitabilityadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/adapters"
	profitabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/usecase"
	publicapiusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/usecase"
	rentaladapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/adapters"
	rentalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	reservationadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/adapters"
	reservationusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	sitemodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site"
	siteusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/usecase"
	smsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	supplieradapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/adapters"
	supplierusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/usecase"
	tenantmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tenant"
	tollsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls"
	tollsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/adapters"
	tollsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/usecase"
	vehiclerecordsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/adapters"
	vehiclerecordsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/repository"
	vehiclerecordsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/usecase"
	vehicleadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/adapters"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	whatsappmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp"
	whatsappusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/observability"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	bulkadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/events"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	ioadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/mail"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx/orgcache"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/outbox"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/queue"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/realtime"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("config load failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logging.New(cfg.Log.Level, cfg.Log.Format)
	flushSentry, err := observability.Init(cfg.Sentry, "worker")
	if err != nil {
		log.Warn("sentry_init_failed", "error", err)
	} else if observability.Enabled() {
		log.Info("sentry_enabled", "environment", cfg.Sentry.Environment, "traces_sample_rate", cfg.Sentry.TracesSampleRate)
	}
	defer flushSentry()
	// exit flushes pending error events before leaving (os.Exit skips defers).
	exit := func(code int) {
		flushSentry()
		os.Exit(code)
	}
	log = slog.New(observability.NewSlogHandler(log.Handler()))
	if !cfg.Queue.Enabled {
		log.Error("queue_disabled")
		exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		log.Error("database_init_failed", "error", err)
		exit(1)
	}
	defer pool.Close()

	store, err := storage.NewFromConfig(ctx, cfg.Storage)
	if err != nil {
		log.Error("storage_init_failed", "error", err)
		exit(1)
	}

	var publisher realtime.Publisher
	if cfg.Centrifugo.Enabled {
		publisher = realtime.NewCentrifugoClient(cfg.Centrifugo)
	} else {
		publisher = realtime.NoopPublisher{}
	}

	queries := database.NewQueries(pool)
	mailer := mail.NewSMTPSender(cfg.SMTP)
	provs := []providers.Provider{
		providers.InappProvider{},
		providers.EmailProvider{Mail: mailer},
		providers.RealtimeProvider{Pub: publisher},
		providers.NoopProvider{Name: "sms", Log: log},
		providers.NoopProvider{Name: "push", Log: log},
	}
	notifSvc := notifusecase.New(queries, nil, provs, log)
	notifSvc.WithVAPID(notifusecase.VAPIDConfig{
		PublicKey:  cfg.VAPID.PublicKey,
		PrivateKey: cfg.VAPID.PrivateKey,
		Subject:    cfg.VAPID.Subject,
	})
	notifSvc.WithExpo(notifusecase.ExpoConfig{AccessToken: cfg.Expo.AccessToken})

	eventBus := events.NewBus(log)
	notifmodule.RegisterEventHandlers(eventBus, notifSvc, log)
	outboxStore := outbox.NewStore(pool, queries)
	outboxPub := outbox.NewPublisher(outboxStore, eventBus, log)
	outboxStop := outboxPub.StartRun(ctx)
	defer outboxStop()

	activityRec := activity.NewRecorder(queries, log)
	// Tenant resources: adapters read the organization from the job row.
	branchSvc := branchusecase.New(branchrepository.NewPostgres(queries), nil)
	fleetSvc := fleetusecase.New(fleetrepository.NewPostgres(pool, queries), store, nil)
	vehicleSvc := vehicleusecase.New(vehiclerepository.NewPostgres(pool, queries), nil)
	// Vehicle documents / maintenance: exports and the daily reminder job.
	vehicleRecordsSvc := vehiclerecordsusecase.New(
		vehiclerecordsrepository.NewPostgres(queries), vehicleSvc, store, notifSvc, log,
	)
	fleetOpsSvc := fleetopsusecase.New(fleetopsrepository.NewPostgres(pool, queries), vehicleSvc)
	// Customers: export (masked unless include_sensitive with reveal), import.
	fieldCipher, err := crypto.NewFieldCipher(cfg.Encryption.Key)
	if err != nil {
		log.Error("field_encryption_init_failed", "error", err)
		exit(1)
	}
	customerSvc := customerusecase.New(customerrepository.NewPostgres(pool, queries), fieldCipher, store, nil, log)
	pricingSvc := pricingusecase.New(
		pricingrepository.NewPostgres(pool, queries),
		orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, queries)),
	)
	// Availability engine: the minute hold expiry sweep.
	availabilitySvc := availabilityusecase.New(pool, queries, vehicleSvc,
		orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, queries)))
	// Extras (equipment hold sweep, export, bulk) and campaigns (export,
	// bulk).
	extrasSvc := extrasusecase.New(pool, queries)
	campaignsSvc := campaignsusecase.New(pool, queries)
	// Reservations: export, bulk cancel / assign, the review / payment
	// window sweep; blocks created by jobs re-plan affected reservations.
	reservationSvc := reservationusecase.New(pool, queries, availabilitySvc, pricingSvc,
		orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, queries)))
	reservationSvc.SetEquipment(reservationadapters.NewEquipment(extrasSvc))
	reservationSvc.SetCoupons(reservationadapters.NewCoupons(campaignsSvc))
	fleetOpsSvc.SetAffectedHandler(reservationadapters.NewAffected(reservationSvc))
	// Payments: the reconciliation sweep retrieves checkouts whose webhook
	// never arrived (runs with the minute hold expiry).
	integrationVault, err := integrations.NewVault(fieldCipher)
	if err != nil {
		log.Error("integration_vault_init_failed", "error", err)
		exit(1)
	}
	iyzicoAdapter := iyzico.New(iyzico.Config{
		SandboxBaseURL: cfg.Payments.IyzicoSandboxBaseURL, LiveBaseURL: cfg.Payments.IyzicoLiveBaseURL,
	})
	// SMS (TEC-734) and the integration job runner (TEC-751): the worker
	// runs the steps of the integrations queues and the minute sweep.
	smsProviders := smsmodule.Providers(cfg.SMS, cfg.App.Env, log)
	// Toll providers (TEC-725): the fake outside production only.
	tollProviders := tollsmodule.Providers(cfg.App.Env, log)
	// KABİS adapters (TEC-726): the official slot and the fake outside
	// production only.
	kabisProviders := kabismodule.Providers(cfg.App.Env, cfg.Guard.KabisAllowProduction, log)
	integrationAdapters := append([]integrations.Adapter{iyzicoAdapter}, smsmodule.Adapters(smsProviders)...)
	integrationAdapters = append(integrationAdapters, tollsmodule.Adapters(tollProviders)...)
	integrationAdapters = append(integrationAdapters, kabismodule.Adapters(kabisProviders)...)
	// E-invoice integrators (TEC-722): the official slot and the fake
	// outside production only.
	invoiceProviders := invoicesmodule.Providers(cfg.App.Env, cfg.Guard.EInvoiceAllowProduction, log)
	integrationAdapters = append(integrationAdapters, invoicesmodule.Adapters(invoiceProviders)...)
	whatsappProviders, whatsappFake := whatsappmodule.Providers(cfg.WhatsApp, cfg.App.Env, log)
	integrationAdapters = append(integrationAdapters, whatsappmodule.Adapters(whatsappProviders)...)
	// Channel managers (TEC-736): the sync sweep runs here.
	channelProviders := channelsmodule.Providers(cfg.App.Env, log)
	integrationAdapters = append(integrationAdapters, channelsmodule.Adapters(channelProviders)...)
	integrationAccountsSvc := integrationaccountsusecase.New(queries, integrationVault,
		integrations.NewRegistry(integrationAdapters...))
	integrationOps := integrations.NewOperationRegistry()
	integrationRunner := integrationjobsusecase.NewRunner(queries, integrationAccountsSvc, integrationOps)
	integrationRunner.SetLogger(log)
	smsSvc, err := smsusecase.New(pool, queries, fieldCipher, integrationAccountsSvc, integrationRunner, integrationOps, smsProviders...)
	if err != nil {
		log.Error("sms_init_failed", "error", err)
		exit(1)
	}
	smsSvc.SetActivity(activityRec)
	smsSvc.SetLogger(log)
	// Redis (organization cache invalidation, plan feature cache); the
	// worker runs without it.
	workerRDB, err := cache.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		log.Warn("worker_redis_unavailable", "error", err)
		workerRDB = nil
	} else {
		defer func() { _ = workerRDB.Close() }()
	}
	// Plan features (TEC-674): SMS / WhatsApp sends, channel syncs, toll
	// fetches and KABİS records skip organizations whose plan lacks the
	// feature; cached per organization in Redis like the API.
	featureStore := features.NewStore(queries)
	var featurePolicy features.AccessPolicy = features.LoaderPolicy{Loader: featureStore}
	var featureSource features.Source = featureStore
	if workerRDB != nil {
		featureCache := orgcache.NewFeatures(featureStore, workerRDB, cfg.Tenancy.CacheTTL, cfg.App.Env)
		featurePolicy, featureSource = featureCache, featureCache
	}
	// Notification engine (TEC-733): reminders, sweep-detected events and
	// due dispatches (minute sweep); SMS delivery reports run the fallback.
	notifyEngine := notifyengineusecase.New(pool, queries, cfg.Auth.FrontendURL)
	notifyEngine.SetActivity(activityRec)
	notifyEngine.SetLogger(log)
	notifyEngine.SetFeatures(featurePolicy)
	notifySMS := notifyengineadapters.NewSMS(smsSvc, queries)
	notifyEngine.Register(notifySMS, notifyengineadapters.NewEmail(mailer),
		notifyengineadapters.NewPush(notifSvc, queries, notifyengineadapters.VAPID{
			PublicKey: cfg.VAPID.PublicKey, PrivateKey: cfg.VAPID.PrivateKey, Subject: cfg.VAPID.Subject,
		}))
	integrationRunner.AddListener(notifySMS)
	// WhatsApp (TEC-735): the engine channel, the health sweep (drop
	// alert, reconnect, receipt timeout).
	whatsappSvc := whatsappusecase.New(queries, integrationAccountsSvc, integrationVault,
		whatsappusecase.Config{WebhookBaseURL: cfg.WhatsApp.WebhookBaseURL}, whatsappProviders...)
	whatsappSvc.SetActivity(activityRec)
	whatsappSvc.SetNotifier(notifSvc)
	whatsappSvc.SetLogger(log)
	if whatsappFake != nil {
		whatsappFake.SetReceipts(whatsappSvc.ApplyReceipt)
	}
	notifyEngine.Register(whatsappusecase.NewChannel(whatsappSvc))
	reservationSvc.SetTransitionNotifier(notifyEngine)
	paymentsSvc := paymentsusecase.New(pool, queries, integrationAccountsSvc, reservationSvc,
		paymentsusecase.Config{PublicBaseURL: cfg.Auth.FrontendURL}, iyzicoAdapter)
	paymentsSvc.SetLogger(log)
	reservationSvc.SetPaymentLinks(paymentsSvc)
	// Contracts: the PDF render task (headless Chromium). Rentals: export,
	// the overdue sweep, reservation lifecycle (bulk cancellations).
	contractsSvc := contractsusecase.New(pool, queries, orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, queries)), store)
	pdfRenderer := pdfrender.NewChromium()
	defer pdfRenderer.Close()
	contractsSvc.SetRenderer(pdfRenderer)
	rentalsSvc := rentalusecase.New(pool, queries, availabilitySvc, rentaladapters.NewReservations(reservationSvc), vehicleSvc)
	rentalsSvc.SetDocuments(contractsSvc)
	// Repeat-customer campaigns (min_completed_rentals) count the
	// customer's returned rentals.
	campaignsSvc.SetRentalCounter(rentalsSvc)
	// Finance: exports / imports / bulk of the ledger and cash book, the
	// catch-up posting sweep; rentals and payments post here too.
	financeSvc := financeusecase.New(pool, queries, store)
	financeSvc.SetLogger(log)
	// Corporate agreements (TEC-723): list export; the periodic billing
	// sweep posts through finance (WithAgreementPeriods below).
	agreementsSvc := agreementusecase.New(pool, queries, financeSvc, pricingSvc)
	rentalsSvc.SetLedger(financeSvc)
	paymentsSvc.SetLedger(financeSvc)
	// Suppliers (TEC-721): exports, imports and bulk archive.
	supplierSvc := supplierusecase.New(pool, queries, financeSvc, nil)
	supplierSvc.SetLogger(log)
	// Field tasks (TEC-724): bulk cancellations re-plan delivery tasks,
	// bulk assign runs here.
	fieldTasksSvc := fieldtasksusecase.New(pool, queries)
	fieldTasksSvc.SetTransfers(fleetOpsSvc)
	fieldTasksSvc.SetFinisher(fieldtasksadapters.Finisher{Rentals: rentalsSvc})
	fieldTasksSvc.SetNotifier(notifSvc)
	fieldTasksSvc.SetLogger(log)
	reservationSvc.SetDeliveryQuoter(fieldTasksSvc)
	// Channel manager (TEC-736): inventory push / booking pull sweep;
	// reservation changes made by jobs ask for a push.
	channelsSvc, err := channelusecase.New(pool, queries, fieldCipher, integrationAccountsSvc,
		channeladapters.NewBooker(reservationSvc, customerSvc), channelProviders...)
	if err != nil {
		log.Error("channels_init_failed", "error", err)
		exit(1)
	}
	channelsSvc.SetLogger(log)
	channelsSvc.SetFeatures(featurePolicy)
	// Public booking API (TEC-728): lifecycle events of its reservations
	// (expiry and cancellations run here too) become webhook deliveries;
	// the minute sweep sends them. No booking runs in the worker.
	publicAPISvc, err := publicapiusecase.New(pool, queries, fieldCipher,
		orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, queries)), reservationSvc, customerSvc, nil)
	if err != nil {
		log.Error("public_api_init_failed", "error", err)
		exit(1)
	}
	publicAPISvc.SetLogger(log)
	publicAPISvc.SetFeatures(featurePolicy)
	publicAPISvc.AllowPrivateWebhooks(!strings.EqualFold(strings.TrimSpace(cfg.App.Env), "production"))
	reservationSvc.SetLifecycleListener(reservationusecase.Listeners{rentalsSvc, fieldTasksSvc, publicAPISvc, channelsSvc})
	// Toll passes (TEC-725): fetch steps (runner operations), the hourly
	// fetch schedule and re-match, file imports.
	tollsSvc := tollsusecase.New(pool, queries, integrationAccountsSvc, integrationRunner, integrationOps, tollProviders...)
	tollsSvc.SetLogger(log)
	tollsSvc.SetFeatures(featurePolicy)
	tollsSvc.SetLedger(tollsadapters.Ledger{Finance: financeSvc})
	// KABİS notifications (TEC-726): send / status-check steps (runner
	// operations), the release of waiting notifications in the minute
	// sweep, corrections from reservation changes handled here.
	kabisSvc, err := kabisusecase.New(pool, queries, fieldCipher, integrationRunner, integrationOps, kabisProviders...)
	if err != nil {
		log.Error("kabis_init_failed", "error", err)
		exit(1)
	}
	kabisSvc.SetStore(store)
	kabisSvc.SetLogger(log)
	kabisSvc.SetFeatures(featurePolicy)
	rentalsSvc.SetKabis(kabisSvc)
	// E-invoices (TEC-722): send / status / cancel steps (runner
	// operations), the release of waiting sends in the minute sweep, list
	// export and bulk invoicing.
	invoicesSvc, err := invoiceusecase.New(pool, queries, fieldCipher, orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, queries)),
		integrationAccountsSvc, integrationRunner, integrationOps, invoiceProviders...)
	if err != nil {
		log.Error("invoices_init_failed", "error", err)
		exit(1)
	}
	invoicesSvc.SetStore(store)
	invoicesSvc.SetFeatures(featurePolicy)
	invoicesSvc.SetPDFRenderer(pdfRenderer)
	invoicesSvc.SetLogger(log)
	// Bulk rental group changes go through the engine like the API.
	vehicleSvc.SetMembershipGuard(availabilitySvc)
	availabilitySvc.SetPlanMovedHandler(reservationSvc)
	vehicleBulk := vehicleadapters.NewBulk(vehicleSvc)
	vehicleBulk.SetMaintenance(fleetopsadapters.NewMaintenance(fleetOpsSvc))
	// Platform organizations (export, bulk activate / suspend). Status
	// changes invalidate the API's organization cache when Redis is up.
	jobScopes := orgusecase.New(orgrepository.NewPostgres(pool, queries), nil)
	jobScopes.SetFeatures(featureSource)
	if workerRDB != nil {
		jobScopes.SetInvalidator(orgcache.New(jobScopes, workerRDB, cfg.Tenancy.CacheTTL, cfg.App.Env))
	}
	ioReg := ioengine.NewRegistry(
		ioadapters.NewUsers(queries),
		ioadapters.NewRoles(queries),
		ioadapters.NewNotifications(queries),
		ioadapters.NewActivity(queries),
		branchadapters.NewIO(branchSvc),
		fleetadapters.NewSegmentsIO(fleetSvc),
		fleetadapters.NewGroupsIO(fleetSvc),
		fleetadapters.NewProductsIO(fleetSvc),
		vehicleadapters.NewIO(vehicleSvc),
		vehiclerecordsadapters.NewDocumentsIO(vehicleRecordsSvc),
		vehiclerecordsadapters.NewMaintenanceIO(vehicleRecordsSvc),
		fleetopsadapters.NewTransfersIO(fleetOpsSvc),
		fleetopsadapters.NewOneWayIO(fleetOpsSvc),
		pricingadapters.NewRatePlansIO(pricingSvc),
		pricingadapters.NewDateRulesIO(pricingSvc),
		extrasadapters.NewIO(extrasSvc),
		campaignsadapters.NewIO(campaignsSvc),
		reservationadapters.NewIO(reservationSvc),
		rentaladapters.NewIO(rentalsSvc),
		tollsadapters.NewIO(tollsSvc),
		orgadapters.NewIO(jobScopes),
		customeradapters.NewIO(customerSvc),
		financeadapters.NewAccountsIO(financeSvc),
		financeadapters.NewEntriesIO(financeSvc),
		financeadapters.NewCashAccountsIO(financeSvc),
		financeadapters.NewMovementsIO(financeSvc),
		financeadapters.NewExpensesIO(financeSvc),
		financeadapters.NewSupplierInvoicesIO(financeSvc),
		financeadapters.NewSupplierPaymentsIO(financeSvc),
		supplieradapters.NewIO(supplierSvc),
		financeadapters.NewDepositsIO(financeSvc),
		financeadapters.NewCollectionsIO(financeSvc),
		agreementadapters.NewIO(agreementsSvc),
		invoiceadapters.NewIO(invoicesSvc),
		// Vehicle profitability report (TEC-727).
		profitabilityadapters.NewIO(profitabilityusecase.New(queries)),
	)
	exportSvc := exportusecase.New(queries, store, ioReg, nil, notifSvc, activityRec, log)
	importSvc := importusecase.New(queries, store, ioReg, nil, notifSvc, activityRec, log)
	bulkReg := bulkengine.NewRegistry(
		bulkadapters.NewUsers(queries),
		bulkadapters.NewRoles(queries),
		branchadapters.NewBulk(branchSvc),
		fleetadapters.NewGroupsBulk(fleetSvc),
		fleetadapters.NewProductsBulk(fleetSvc),
		vehicleBulk,
		fleetopsadapters.NewOneWayBulk(fleetOpsSvc),
		extrasadapters.NewBulk(extrasSvc),
		campaignsadapters.NewBulk(campaignsSvc),
		reservationadapters.NewBulk(reservationSvc),
		fieldtasksadapters.NewBulk(fieldTasksSvc),
		orgadapters.NewBulk(jobScopes),
		customeradapters.NewBulk(customerSvc),
		financeadapters.NewMovementsBulk(financeSvc),
		financeadapters.NewExpensesBulk(financeSvc),
		supplieradapters.NewBulk(supplierSvc),
		financeadapters.NewCollectionsBulk(financeSvc),
		invoiceadapters.NewBulk(invoicesSvc),
	)
	bulkSvc := bulkusecase.New(queries, bulkReg, nil, notifSvc, activityRec, cfg.Bulk, log)
	// Tenant jobs run with the requester's current membership (permissions
	// and branch scope), re-resolved from the job's actor.
	exportSvc.SetJobScopeLoader(jobScopes)
	importSvc.SetJobScopeLoader(jobScopes)
	bulkSvc.SetJobScopeLoader(jobScopes)
	logsSvc := logsusecase.New(queries)
	// Steps run inline inside the worker task, so no queue is passed.
	catalogSvc := catalogusecase.New(catalogrepository.NewPostgres(pool, queries), store, nil, activityRec, log)
	searchReg := searchengine.NewRegistry(
		searchadapters.NewUsers(queries),
		searchadapters.NewRoles(queries),
		branchadapters.NewSearch(queries),
		fleetadapters.NewProductsSearch(queries),
		vehicleadapters.NewSearch(queries),
		customeradapters.NewSearch(queries),
		supplieradapters.NewSearch(queries),
	)
	searchClient := searchengine.NewClient(cfg.Search, log)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, nil, log)
	// Vehicle imports / bulk changes keep the Cmd+K index current.
	vehicleSvc.SetSearchIndexer(searchIndexer)
	customerSvc.SetSearchIndexer(searchIndexer)
	supplierSvc.SetSearchIndexer(searchIndexer)
	// Deliveries run inline here; mobile pushes go through the queue (retries).
	queueClient := queue.NewClient(cfg.Redis)
	defer func() { _ = queueClient.Close() }()
	siteBaseURL := cfg.Site.BaseURL
	if siteBaseURL == "" {
		siteBaseURL = siteusecase.DefaultBaseURL
	}
	siteBase, err := siteusecase.ParseBaseURL(siteBaseURL)
	if err != nil {
		log.Error("site_base_invalid", "error", err)
		exit(1)
	}
	siteSvc := siteusecase.New(queries, publicAPISvc, nil, siteBase, cfg.Auth.FrontendURL)
	siteDomains, err := sitemodule.ConfigureDomains(queries, siteSvc, cfg.Site, queueClient.EnqueueSiteDomains)
	if err != nil {
		log.Error("site_domains_init_failed", "error", err)
		exit(1)
	}
	notifSvc.WithPushQueue(queueClient)
	integrationRunner.SetDispatcher(queue.IntegrationDispatcher{Client: queueClient})
	persist := logging.Attach(log, logsSvc)
	log = persist.Logger()
	defer persist.Close()
	// Bulk / import jobs finish here: tell open lists what changed.
	resourceEvents := resourceevents.New(publisher, log)
	defer resourceEvents.Wait()
	// Tenant jobs carry only the organization id: scope their events.
	resourceEvents.AddResolver(tenantmodule.ChannelResolver(tenantmodule.QueriesLookup(queries)))
	catalogSvc.SetResourceEvents(resourceEvents)
	importSvc.SetResourceEvents(resourceEvents)
	bulkSvc.SetResourceEvents(resourceEvents)
	reservationSvc.SetResourceEvents(resourceEvents)
	paymentsSvc.SetResourceEvents(resourceEvents)
	integrationRunner.SetResourceEvents(resourceEvents)
	rentalsSvc.SetResourceEvents(fieldtasksusecase.MirrorPublisher{Publisher: resourceEvents})
	fieldTasksSvc.SetResourceEvents(resourceEvents)
	contractsSvc.SetResourceEvents(resourceEvents)
	financeSvc.SetResourceEvents(resourceEvents)
	tollsSvc.SetResourceEvents(resourceEvents)
	kabisSvc.SetResourceEvents(resourceEvents)
	invoicesSvc.SetResourceEvents(resourceEvents)
	channelsSvc.SetResourceEvents(resourceEvents)

	// Platform health alert sweep (TEC-739): modules with their own
	// sync / delivery health register a healthcheck.Source on
	// healthRegistry here (and in the API's composition root).
	healthRegistry := platformhealthmodule.NewRegistry(queries)
	if err := healthRegistry.Register(siteDomains); err != nil {
		log.Error("domain_health_init_failed", "error", err)
		exit(1)
	}
	// Channel manager sync health (TEC-736): failed syncs, expired pushes,
	// the error queue.
	if err := healthRegistry.Register(channelsmodule.NewHealthSource(queries)); err != nil {
		log.Error("channel_health_init_failed", "error", err)
	}
	platformHealthSvc := platformhealthusecase.New(queries, healthRegistry)
	platformHealthSvc.SetNotifier(notifSvc)
	platformHealthSvc.SetLogger(log)
	// Failed-work alerts (TEC-744): dead tasks and integration jobs that
	// need attention, one per job kind per window (Redis dedupe).
	platformHealthSvc.SetFailureAlerts(platformhealthusecase.FailureAlertConfig{
		Email: cfg.Alerts.Email, Window: cfg.Alerts.DedupeWindow, Dedupe: platformhealthusecase.RedisDeduper{RDB: workerRDB},
	})
	integrationRunner.AddListener(platformHealthSvc)

	worker := queue.NewWorker(cfg, log, notifSvc.Deliver).
		WithExport(exportSvc.ProcessExport).
		WithImport(importSvc.ProcessImport).
		WithBulk(bulkSvc.ProcessBulk).
		WithLogPurge(logsSvc.ApplyDueRules).
		WithMobilePush(notifSvc.SendMobilePush, notifSvc.ProcessPushReceipts).
		WithCatalogImport(catalogSvc.ProcessImport).
		WithVehicleReminders(vehicleRecordsSvc.RunReminders).
		WithExpireHolds(func(ctx context.Context) (int, error) {
			// Reservations whose review / payment window elapsed first
			// (each releases its option once), then vehicle holds (their
			// equipment expires with them via the database trigger), then
			// standalone equipment holds.
			r, err := reservationSvc.ExpireDue(ctx)
			if err != nil {
				return r, err
			}
			n, err := availabilitySvc.ExpireDue(ctx)
			if err != nil {
				return r + n, err
			}
			m, err := extrasSvc.ExpireDue(ctx)
			if err != nil {
				return r + n + m, err
			}
			// Pending checkouts (lost webhooks) after the expiries: a late
			// payment is then re-validated against current capacity.
			p, err := paymentsSvc.ReconcilePending(ctx)
			if err != nil {
				return r + n + m + p, err
			}
			// Late vehicles: keep them busy, re-plan or flag what follows.
			o, err := rentalsSvc.SweepOverdue(ctx)
			if err != nil {
				return r + n + m + p + o, err
			}
			// Confirmed payments / refunds whose ledger entry is missing
			// (posting is idempotent).
			l, err := financeSvc.PostPending(ctx)
			return r + n + m + p + o + l, err
		}).
		WithIntegrations(func(ctx context.Context, orgID, jobID int64) error {
			_, err := integrationRunner.Process(ctx, orgID, jobID, integrationjobsusecase.TriggerWorker)
			if errors.Is(err, integrationjobsusecase.ErrNotFound) || errors.Is(err, integrationjobsusecase.ErrBusy) {
				// Deleted with its organization / another step holds it.
				return nil
			}
			return err
		}, func(ctx context.Context) (int, error) {
			// Waiting KABİS notifications whose organization now has a
			// working adapter, then the due runner steps.
			released, err := kabisSvc.ReleaseWaiting(ctx)
			if err != nil {
				log.Warn("kabis_release_failed", "error", err)
			}
			// Waiting e-invoice sends likewise (TEC-722).
			invoices, err := invoicesSvc.ReleaseWaiting(ctx)
			if err != nil {
				log.Warn("invoice_release_failed", "error", err)
			}
			// Channel managers (TEC-736): due inventory pushes (event or
			// interval) and booking pulls.
			synced, err := channelsSvc.SyncDue(ctx)
			if err != nil {
				log.Warn("channel_sync_failed", "error", err)
			}
			n, err := integrationRunner.RunDue(ctx)
			return released + invoices + synced + n, err
		}).
		WithTolls(tollsSvc.ScheduleFetches, tollsSvc.RematchDue).
		WithAgreementPeriods(financeSvc.AccruePeriods).
		WithPlatformHealthAlerts(platformHealthSvc.RunAlertsCount).
		WithFailureAlerts(platformHealthSvc.AlertDeadTask).
		WithPublicWebhooks(publicAPISvc.DeliverDue).
		WithSiteDomains(siteDomains.Sweep).
		WithNotifyEngine(notifyEngine.Sweep).
		WithWhatsApp(whatsappSvc.Watch).
		WithDocumentPDF(contractsSvc.RenderPDF).
		WithSearch(
			searchIndexer.ProcessUpsert,
			searchIndexer.ProcessDelete,
			searchIndexer.ProcessReindex,
		)

	if searchIndexer.Enabled() {
		go searchIndexer.Bootstrap(ctx)
	}

	scheduler, err := queue.StartLogPurgeScheduler(cfg, log)
	if err != nil {
		log.Error("log_purge_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterPushReceiptsSchedule(scheduler); err != nil {
		log.Error("push_receipts_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterVehicleRemindersSchedule(scheduler); err != nil {
		log.Error("vehicle_reminders_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterIntegrationSweepSchedule(scheduler); err != nil {
		log.Error("integration_sweep_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterTollsSchedule(scheduler); err != nil {
		log.Error("toll_fetch_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterExpireHoldsSchedule(scheduler); err != nil {
		log.Error("hold_expiry_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterAgreementPeriodsSchedule(scheduler); err != nil {
		log.Error("agreement_periods_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterPlatformHealthAlertsSchedule(scheduler); err != nil {
		log.Error("platform_health_alerts_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterSiteDomainsSchedule(scheduler); err != nil {
		log.Error("site_domains_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterPublicWebhooksSchedule(scheduler); err != nil {
		log.Error("public_webhooks_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterNotifyEngineSchedule(scheduler); err != nil {
		log.Error("notify_engine_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterWhatsAppSchedule(scheduler); err != nil {
		log.Error("whatsapp_scheduler_failed", "error", err)
		exit(1)
	}
	go func() {
		if err := scheduler.Run(); err != nil {
			log.Error("log_purge_scheduler_stopped", "error", err)
		}
	}()

	log.Info("worker_started",
		"app", cfg.App.Name,
		"env", cfg.App.Env,
		"concurrency", cfg.Queue.Concurrency,
	)

	if n, err := notifSvc.ReclaimStuck(ctx, notifusecase.DefaultStuckProcessingMinutes); err != nil {
		log.Error("notification_reclaim_failed", "error", err)
	} else if n > 0 {
		log.Info("notification_reclaim_completed", "count", n)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- worker.Start()
	}()

	select {
	case <-ctx.Done():
		log.Info("worker_shutdown_signal")
		worker.Shutdown()
	case err := <-errCh:
		if err != nil {
			log.Error("worker_failed", "error", err)
			exit(1)
		}
	}
	log.Info("worker_stopped")
}
