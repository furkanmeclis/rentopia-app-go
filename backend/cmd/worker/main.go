package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/cache"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/logging"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/adapters"
	branchrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	bulkusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/usecase"
	campaignsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/adapters"
	campaignsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	catalogrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	catalogusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/usecase"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	customeradapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	customerrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	customerusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	exportusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/exports/usecase"
	extrasadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/adapters"
	extrasusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/usecase"
	fieldtasksadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/adapters"
	fieldtasksusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/usecase"
	fleetadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/adapters"
	fleetrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/repository"
	fleetusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/usecase"
	fleetopsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/adapters"
	fleetopsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	fleetopsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	importusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/imports/usecase"
	integrationaccountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	integrationjobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	logsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/logs/usecase"
	notifmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/usecase"
	orgadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/adapters"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	pricingadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/adapters"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	rentaladapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/adapters"
	rentalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	reservationadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/adapters"
	reservationusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	smsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/observability"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	bulkadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/events"
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
	integrationAdapters := append([]integrations.Adapter{iyzicoAdapter}, smsmodule.Adapters(smsProviders)...)
	integrationAdapters = append(integrationAdapters, tollsmodule.Adapters(tollProviders)...)
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
	// Field tasks (TEC-724): bulk cancellations re-plan delivery tasks,
	// bulk assign runs here.
	fieldTasksSvc := fieldtasksusecase.New(pool, queries)
	fieldTasksSvc.SetTransfers(fleetOpsSvc)
	fieldTasksSvc.SetFinisher(fieldtasksadapters.Finisher{Rentals: rentalsSvc})
	fieldTasksSvc.SetNotifier(notifSvc)
	fieldTasksSvc.SetLogger(log)
	reservationSvc.SetDeliveryQuoter(fieldTasksSvc)
	reservationSvc.SetLifecycleListener(reservationusecase.Listeners{rentalsSvc, fieldTasksSvc})
	// Toll passes (TEC-725): fetch steps (runner operations), the hourly
	// fetch schedule and re-match, file imports.
	tollsSvc := tollsusecase.New(pool, queries, integrationAccountsSvc, integrationRunner, integrationOps, tollProviders...)
	tollsSvc.SetLogger(log)
	// Bulk rental group changes go through the engine like the API.
	vehicleSvc.SetMembershipGuard(availabilitySvc)
	availabilitySvc.SetPlanMovedHandler(reservationSvc)
	vehicleBulk := vehicleadapters.NewBulk(vehicleSvc)
	vehicleBulk.SetMaintenance(fleetopsadapters.NewMaintenance(fleetOpsSvc))
	// Platform organizations (export, bulk activate / suspend). Status
	// changes invalidate the API's organization cache when Redis is up.
	jobScopes := orgusecase.New(orgrepository.NewPostgres(pool, queries), nil)
	if rdb, err := cache.NewRedisClient(ctx, cfg.Redis); err == nil {
		defer func() { _ = rdb.Close() }()
		jobScopes.SetInvalidator(orgcache.New(jobScopes, rdb, cfg.Tenancy.CacheTTL, cfg.App.Env))
	} else {
		log.Warn("worker_redis_unavailable", "error", err)
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
	)
	searchClient := searchengine.NewClient(cfg.Search, log)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, nil, log)
	// Vehicle imports / bulk changes keep the Cmd+K index current.
	vehicleSvc.SetSearchIndexer(searchIndexer)
	customerSvc.SetSearchIndexer(searchIndexer)
	// Deliveries run inline here; mobile pushes go through the queue (retries).
	queueClient := queue.NewClient(cfg.Redis)
	defer func() { _ = queueClient.Close() }()
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
	tollsSvc.SetResourceEvents(resourceEvents)

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
			return r + n + m + p + o, err
		}).
		WithIntegrations(func(ctx context.Context, orgID, jobID int64) error {
			_, err := integrationRunner.Process(ctx, orgID, jobID, integrationjobsusecase.TriggerWorker)
			if errors.Is(err, integrationjobsusecase.ErrNotFound) || errors.Is(err, integrationjobsusecase.ErrBusy) {
				// Deleted with its organization / another step holds it.
				return nil
			}
			return err
		}, integrationRunner.RunDue).
		WithTolls(tollsSvc.ScheduleFetches, tollsSvc.RematchDue).
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
