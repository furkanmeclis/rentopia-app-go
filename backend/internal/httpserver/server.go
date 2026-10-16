package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/cache"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	accessmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/access"
	accesshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/access/handler"
	activitymodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/activity"
	activityhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/activity/handler"
	activityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/activity/usecase"
	agreementsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements"
	agreementadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/adapters"
	agreementhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/handler"
	agreementusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/usecase"
	authmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth"
	authhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/identity"
	authrepo "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/repository"
	authusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/usecase"
	authsettingsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/authsettings"
	authsettingshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/authsettings/handler"
	authsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/authsettings/usecase"
	availabilitymodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability"
	availabilityhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/handler"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchesmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches"
	branchadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/adapters"
	branchhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/handler"
	branchrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	bulkmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk"
	bulkhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/handler"
	bulkusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/usecase"
	campaignsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns"
	campaignsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/adapters"
	campaignshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/handler"
	campaignsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	catalogmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog"
	cataloghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/handler"
	catalogrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	catalogusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/usecase"
	channelsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels"
	channeladapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/adapters"
	channelhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/handler"
	channelusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/usecase"
	contractsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts"
	contractshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/handler"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	customerportalmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal"
	customerportalhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/handler"
	customerportalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/usecase"
	customersmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers"
	customeradapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	customerhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/handler"
	customerrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	customerusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	exportmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/exports"
	exporthandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/exports/handler"
	exportusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/exports/usecase"
	extrasmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras"
	extrasadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/adapters"
	extrashandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/handler"
	extrasusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/usecase"
	fieldtasksmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks"
	fieldtasksadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/adapters"
	fieldtaskshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/handler"
	fieldtasksusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/usecase"
	financemodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance"
	financeadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/adapters"
	financehandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/handler"
	financeusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	fleetmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet"
	fleetadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/adapters"
	fleethandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/handler"
	fleetrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/repository"
	fleetusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/usecase"
	fleetopsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops"
	fleetopsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/adapters"
	fleetopshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/handler"
	fleetopsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	fleetopsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	importmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/imports"
	importhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/imports/handler"
	importusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/imports/usecase"
	inspectionsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections"
	inspectionshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/handler"
	inspectionsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/usecase"
	integrationaccountsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts"
	integrationaccountshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/handler"
	integrationaccountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	githubmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/github"
	githubhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/github/handler"
	githubusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/github/usecase"
	integrationjobsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs"
	integrationjobshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/handler"
	integrationjobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	oauthprovidermodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/oauthprovider"
	oauthproviderhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/oauthprovider/handler"
	oauthproviderusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/oauthprovider/usecase"
	invoicesmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices"
	invoiceadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/adapters"
	invoicehandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/handler"
	invoiceusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/usecase"
	kabismodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis"
	kabishandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/handler"
	kabisusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/usecase"
	logsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/logs"
	logshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/logs/handler"
	logsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/logs/usecase"
	notifmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications"
	notifhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/usecase"
	notifyenginemodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine"
	notifyengineadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/adapters"
	notifyenginehandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/handler"
	notifyengineusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/usecase"
	organizationsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations"
	orgadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/adapters"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings"
	orgsettingshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/handler"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	overviewmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/overview"
	overviewhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/overview/handler"
	overviewusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/overview/usecase"
	paymentsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments"
	paymentshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/handler"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	platformhealthmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth"
	platformhealthhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/handler"
	platformhealthusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/usecase"
	pricingmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing"
	pricingadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/adapters"
	pricinghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/handler"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	profitabilitymodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability"
	profitabilityadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/adapters"
	profitabilityhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/handler"
	profitabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/usecase"
	publicapimodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi"
	publicapihandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/handler"
	publicapiusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/qrlogin"
	rentalsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals"
	rentaladapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/adapters"
	rentalhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/handler"
	rentalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	reportsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports"
	reportsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/adapters"
	reportshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/handler"
	reportsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/usecase"
	reservationsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations"
	reservationadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/adapters"
	reservationhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/handler"
	reservationusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	searchmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/search"
	searchhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/search/handler"
	searchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/search/usecase"
	settingsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/settings"
	settingshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/settings/handler"
	settingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/settings/usecase"
	smsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms"
	smshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/handler"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	storagemodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/storage"
	storagehandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/storage/handler"
	storageusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/storage/usecase"
	suppliersmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers"
	supplieradapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/adapters"
	supplierhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/handler"
	supplierusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/usecase"
	tenantmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tenant"
	tollsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls"
	tollsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/adapters"
	tollshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/handler"
	tollsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/usecase"
	vehiclerecordsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords"
	vehiclerecordsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/adapters"
	vehiclerecordshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/handler"
	vehiclerecordsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/repository"
	vehiclerecordsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/usecase"
	vehiclesmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles"
	vehicleadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/adapters"
	vehiclehandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/handler"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/appleauth"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	bulkadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/events"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	ioadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/mail"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/oidc"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx/orgcache"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/outbox"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/stepup"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/queue"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/realtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Deps holds infrastructure clients wired into the HTTP server.
type Deps struct {
	DB       *pgxpool.Pool
	Queries  *db.Queries
	Redis    *redis.Client
	Queue    *queue.Client
	Storage  storage.Driver
	Realtime realtime.Publisher
	Worker   *queue.Worker
	Events   events.Bus
}

// Server is the HTTP composition root for infrastructure routes.
type Server struct {
	cfg           config.Config
	log           *slog.Logger
	db            *pgxpool.Pool
	queries       *db.Queries
	redis         *redis.Client
	queueClient   *queue.Client
	storage       storage.Driver
	realtime      realtime.Publisher
	worker        *queue.Worker
	events        events.Bus
	outboxPub     *outbox.Publisher
	outboxStop    func()
	searchClient  *searchengine.Client
	searchIndexer *searchengine.Indexer
	githubSvc     *githubusecase.Service
	oauthProvSvc  *oauthproviderusecase.Service
	authSettings  *authsettingsusecase.Service
	mux           *http.ServeMux
	http          *http.Server
}

// New wires router and middleware for the API skeleton.
func New(cfg config.Config, log *slog.Logger, deps Deps) (*Server, error) {
	mux := http.NewServeMux()
	eventBus := deps.Events
	if eventBus == nil {
		eventBus = events.NewBus(log)
	}

	s := &Server{
		cfg:         cfg,
		log:         log,
		db:          deps.DB,
		queries:     deps.Queries,
		redis:       deps.Redis,
		queueClient: deps.Queue,
		storage:     deps.Storage,
		realtime:    deps.Realtime,
		worker:      deps.Worker,
		events:      eventBus,
		mux:         mux,
	}

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /v1/app/config", s.handleAppConfig)
	s.mountDocs(mux)

	tokens, err := jwt.NewManager(cfg.JWT.AccessSecret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	if err != nil {
		return nil, fmt.Errorf("httpserver: jwt: %w", err)
	}

	mailer := mail.NewSMTPSender(cfg.SMTP)
	provs := []providers.Provider{
		providers.InappProvider{},
		providers.EmailProvider{Mail: mailer},
		providers.RealtimeProvider{Pub: deps.Realtime},
		providers.NoopProvider{Name: "sms", Log: log},
		providers.NoopProvider{Name: "push", Log: log},
	}
	// Job queue of the engines; nil (no Redis queue) runs jobs inline. A nil
	// *queue.Client must not reach the use cases as a non-nil interface.
	var jobQueue exportusecase.Enqueuer
	if deps.Queue != nil {
		jobQueue = deps.Queue
	}
	notifSvc := notifusecase.New(deps.Queries, jobQueue, provs, log)
	notifSvc.WithActionSigner(cfg.JWT.AccessSecret, 0)
	notifSvc.WithVAPID(notifusecase.VAPIDConfig{
		PublicKey:  cfg.VAPID.PublicKey,
		PrivateKey: cfg.VAPID.PrivateKey,
		Subject:    cfg.VAPID.Subject,
	})
	notifSvc.WithExpo(notifusecase.ExpoConfig{AccessToken: cfg.Expo.AccessToken})

	repo := authrepo.NewPostgres(deps.DB, deps.Queries)
	uc := authusecase.New(repo, tokens)
	uc.SetNotifier(notifSvc)
	uc.SetLogger(log)

	searchReg := searchengine.NewRegistry(
		searchadapters.NewUsers(deps.Queries),
		searchadapters.NewRoles(deps.Queries),
		branchadapters.NewSearch(deps.Queries),
		fleetadapters.NewProductsSearch(deps.Queries),
		vehicleadapters.NewSearch(deps.Queries),
		customeradapters.NewSearch(deps.Queries),
		supplieradapters.NewSearch(deps.Queries),
	)
	searchClient := searchengine.NewClient(cfg.Search, log)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, deps.Queue, log)
	s.searchClient = searchClient
	s.searchIndexer = searchIndexer
	uc.SetSearchIndexer(searchIndexer)

	// Live DataTables: resource-change events on "resource:{name}" channels;
	// subscriptions need the read permission each module registers.
	resourceEvents := resourceevents.New(deps.Realtime, log)
	liveResources := resourceevents.NewRegistry()
	authmodule.RegisterLiveResources(liveResources)
	uc.SetResourceEvents(resourceEvents)
	uc.AddChannelRules(liveResources.ChannelRule())

	rtIssuer, err := realtime.NewTokenIssuer(cfg.Centrifugo)
	if err != nil {
		return nil, fmt.Errorf("httpserver: realtime tokens: %w", err)
	}
	uc.SetRealtimeHints(authusecase.RealtimeHints{
		Enabled: rtIssuer.Enabled(),
		WSURL:   cfg.Centrifugo.WSURL,
	})

	activityRec := activity.NewRecorder(deps.Queries, log)

	stepUpStore := stepup.NewStore(deps.Redis, cfg.App.Env)
	var stepUpWebAuthn *stepup.WebAuthn
	stepUpOrigins := cfg.CORS.AllowedOrigins
	if len(stepUpOrigins) == 0 {
		stepUpOrigins = []string{strings.TrimRight(cfg.Auth.FrontendURL, "/")}
	}
	if webauthn, err := stepup.NewWebAuthn(stepup.WebAuthnConfig{
		RPID:          cfg.Auth.WebAuthnRPID,
		RPDisplayName: cfg.App.Name,
		RPOrigins:     stepUpOrigins,
	}); err == nil {
		stepUpWebAuthn = webauthn
	} else {
		log.Warn("stepup_webauthn_disabled", "error", err)
	}
	secretBox, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		return nil, fmt.Errorf("httpserver: encryption: %w", err)
	}
	// Column-level encryption of personal data (customers, drivers): keys
	// derived from APP_ENCRYPTION_KEY, separate from the SecretBox key.
	fieldCipher, err := crypto.NewFieldCipher(cfg.Encryption.Key)
	if err != nil {
		return nil, fmt.Errorf("httpserver: field encryption: %w", err)
	}
	stepUpSvc := stepup.NewService(deps.Queries, stepUpStore, stepUpWebAuthn, repo)
	stepUpSvc.SetSecretBox(secretBox)

	githubSvc := githubusecase.New(deps.Queries, secretBox)
	oauthProvSvc := oauthproviderusecase.New(deps.Queries, secretBox)
	authSettingsSvc := authsettingsusecase.New(deps.Queries)
	uc.SetAuthSettings(authSettingsSvc)
	uc.SetAccessPolicy(stepUpSvc)
	uc.SetSecretBox(secretBox, cfg.App.Name)
	oauthUC := authusecase.NewOAuth(repo, secretBox)
	uc.SetReviewAccounts(cfg.Auth.ReviewAccounts)
	// Apple signing key: admin-uploaded .p8 (DB) first, AUTH_APPLE_* env fallback.
	appleClient, err := appleauth.NewResolver(oauthProvSvc, appleauth.Config{
		TeamID: cfg.Auth.AppleTeamID, KeyID: cfg.Auth.AppleKeyID, PrivateKey: cfg.Auth.ApplePrivateKey,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("httpserver: apple sign-in key: %w", err)
	}
	oauthProvSvc.SetAppleKeys(appleClient)
	uc.SetNativeOAuth(authusecase.NativeOAuthConfig{
		Verifier:        oidc.New(oidc.DefaultProviders(), nil),
		Apple:           appleClient,
		Clients:         oauthProvSvc,
		AppleClientIDs:  cfg.Auth.AppleNativeClientIDs,
		GoogleClientIDs: cfg.Auth.GoogleNativeClientIDs,
	})
	s.githubSvc = githubSvc
	s.oauthProvSvc = oauthProvSvc
	s.authSettings = authSettingsSvc

	h := authhandler.New(uc, oauthUC, githubSvc, oauthProvSvc, authSettingsSvc, cfg.Auth.AdapterSecret, stepUpSvc, activityRec)
	h.SetRateLimiter(ratelimit.New(deps.Redis, cfg.App.Env))
	loader := identity.Loader{UC: uc}
	authmodule.RegisterRoutes(mux, h, tokens, loader, stepUpSvc)

	// Organizations (tenant core). Platform management is always mounted;
	// the /v1/tenant/* shell only with TENANCY_ENABLED.
	// Tenant resources (organization-scoped). OrganizationHooks seed the
	// default branch of every new organization.
	branchSvc := branchusecase.New(branchrepository.NewPostgres(deps.Queries), searchIndexer)
	// Fleet classification: segments, rental groups, versioned products.
	fleetSvc := fleetusecase.New(fleetrepository.NewPostgres(deps.DB, deps.Queries), deps.Storage, searchIndexer)
	// Fleet vehicles (physical reality, status history, group memberships).
	vehicleSvc := vehicleusecase.New(vehiclerepository.NewPostgres(deps.DB, deps.Queries), searchIndexer)
	// Vehicle documents, maintenance, expiry alerts and reminders (TEC-693).
	vehicleRecordsSvc := vehiclerecordsusecase.New(
		vehiclerecordsrepository.NewPostgres(deps.Queries), vehicleSvc, deps.Storage, notifSvc, log,
	)
	// Customers and drivers (TEC-705): encrypted identity / licence values.
	customerSvc := customerusecase.New(
		customerrepository.NewPostgres(deps.DB, deps.Queries), fieldCipher, deps.Storage, searchIndexer, log,
	)
	// Fleet operations: vehicle blocks, transfers, one-way rules.
	fleetOpsSvc := fleetopsusecase.New(fleetopsrepository.NewPostgres(deps.DB, deps.Queries), vehicleSvc)
	orgSvc := orgusecase.New(
		orgrepository.NewPostgres(deps.DB, deps.Queries),
		// New organizations get the "Merkez" branch and the default
		// expense categories.
		orgusecase.ChainHooks{branchusecase.NewOrganizationHooks(searchIndexer), financeusecase.OrganizationHooks{}},
	)
	uc.SetOrganizationsLister(orgSvc)
	// Staff invitations, organization logo and the platform management views.
	orgMgmt := orgusecase.NewManagement(orgSvc, orgrepository.NewPostgres(deps.DB, deps.Queries), orgusecase.ManagementConfig{
		FrontendURL: cfg.Auth.FrontendURL,
	})
	orgMgmt.SetNotifier(notifSvc)
	orgMgmt.SetAccounts(uc)
	orgMgmt.SetRateLimiter(ratelimit.New(deps.Redis, cfg.App.Env))
	orgMgmt.SetLogger(log)
	orgH := orghandler.New(orgSvc, activityRec)
	orgH.SetManagement(orgMgmt)
	orgH.SetStorage(deps.Storage)
	orgH.SetRegistrationLimiter(ratelimit.New(deps.Redis, cfg.App.Env))
	orgH.SetResourceEvents(resourceEvents)
	organizationsmodule.RegisterPlatformLiveResources(liveResources)
	var tenantResolver orgctx.Resolver
	if cfg.Tenancy.Enabled {
		orgCache := orgcache.New(orgSvc, deps.Redis, cfg.Tenancy.CacheTTL, cfg.App.Env)
		orgSvc.SetInvalidator(orgCache)
		tenantResolver = orgCache
	}
	tenantmodule.RegisterRoutes(mux, tokens, loader, tenantResolver)
	organizationsmodule.RegisterTenantRoutes(mux, orgH, tokens, loader, tenantResolver)
	orgSettingsSvc := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(deps.DB, deps.Queries))
	orgsettingsmodule.RegisterRoutes(mux, orgsettingshandler.New(orgSettingsSvc, activityRec), tokens, loader, tenantResolver)
	// Pricing (TEC-700 / TEC-701): rate plans, date rules, preview.
	pricingSvc := pricingusecase.New(pricingrepository.NewPostgres(deps.DB, deps.Queries), orgSettingsSvc)
	pricingSvc.SetOneWayResolver(pricingadapters.NewOneWay(fleetOpsSvc))
	// Extras (TEC-702) and campaigns (TEC-703) feed the engine's extras and
	// discount hooks.
	extrasSvc := extrasusecase.New(deps.DB, deps.Queries)
	campaignsSvc := campaignsusecase.New(deps.DB, deps.Queries)
	pricingSvc.SetExtrasResolver(pricingadapters.NewExtras(extrasSvc))
	pricingSvc.SetCouponEvaluator(pricingadapters.NewCoupons(campaignsSvc))
	// Availability engine (TEC-697): capacity search, holds, channel rules.
	availabilitySvc := availabilityusecase.New(deps.DB, deps.Queries, vehicleSvc, orgSettingsSvc)
	// Reservations (TEC-704 / TEC-706 / TEC-698): quotes, the state machine
	// on the engine, plate plan, resolution tasks. Equipment (TEC-702) and
	// campaigns (TEC-703) plug in through narrow ports; blocks over sold
	// capacity re-plan reservations or open resolution tasks.
	reservationSvc := reservationusecase.New(deps.DB, deps.Queries, availabilitySvc, pricingSvc, orgSettingsSvc)
	reservationSvc.SetEquipment(reservationadapters.NewEquipment(extrasSvc))
	reservationSvc.SetCoupons(reservationadapters.NewCoupons(campaignsSvc))
	reservationSvc.SetResourceEvents(resourceEvents)
	fleetOpsSvc.SetAffectedHandler(reservationadapters.NewAffected(reservationSvc))
	// Integrations (TEC-676) and payments (TEC-707 / TEC-708): each
	// organization's own provider account (credentials sealed with the
	// field cipher under their own purpose), pay-by-link + hosted
	// checkout, verified webhooks; payments tell reservations through the
	// PaymentRecorder port, staff accept creates links through
	// PaymentLinker.
	integrationVault, err := integrations.NewVault(fieldCipher)
	if err != nil {
		return nil, fmt.Errorf("httpserver: integration vault: %w", err)
	}
	iyzicoAdapter := iyzico.New(iyzico.Config{
		SandboxBaseURL: cfg.Payments.IyzicoSandboxBaseURL, LiveBaseURL: cfg.Payments.IyzicoLiveBaseURL,
	})
	// SMS providers (TEC-734): Netgsm, and the log provider in development.
	smsProviders := smsmodule.Providers(cfg.SMS, cfg.App.Env, log)
	// Toll providers (TEC-725): none is chosen yet (TEC-683); the fake is
	// registered outside production only.
	tollProviders := tollsmodule.Providers(cfg.App.Env, log)
	// KABİS adapters (TEC-726): the official slot (waiting on dependency
	// until TEC-682) and the fake outside production only.
	kabisProviders := kabismodule.Providers(cfg.App.Env, log)
	integrationAdapters := append([]integrations.Adapter{iyzicoAdapter}, smsmodule.Adapters(smsProviders)...)
	integrationAdapters = append(integrationAdapters, tollsmodule.Adapters(tollProviders)...)
	integrationAdapters = append(integrationAdapters, kabismodule.Adapters(kabisProviders)...)
	// E-invoice integrators (TEC-722): the official slot (waiting on
	// dependency until TEC-685) and the fake outside production only.
	invoiceProviders := invoicesmodule.Providers(cfg.App.Env, log)
	integrationAdapters = append(integrationAdapters, invoicesmodule.Adapters(invoiceProviders)...)
	// Channel managers (TEC-736): Yolcu360 (documented supplier contract)
	// and its fake outside production only.
	channelProviders := channelsmodule.Providers(cfg.App.Env, log)
	integrationAdapters = append(integrationAdapters, channelsmodule.Adapters(channelProviders)...)
	integrationAccountsSvc := integrationaccountsusecase.New(deps.Queries, integrationVault,
		integrations.NewRegistry(integrationAdapters...))
	// Generic integration job runner (TEC-751): steps go to the
	// integrations queues when a queue is configured.
	integrationOps := integrations.NewOperationRegistry()
	integrationRunner := integrationjobsusecase.NewRunner(deps.Queries, integrationAccountsSvc, integrationOps)
	if deps.Queue != nil {
		integrationRunner.SetDispatcher(queue.IntegrationDispatcher{Client: deps.Queue})
	}
	integrationRunner.SetResourceEvents(resourceEvents)
	integrationRunner.SetLogger(log)
	smsSvc, err := smsusecase.New(deps.DB, deps.Queries, fieldCipher, integrationAccountsSvc, integrationRunner, integrationOps, smsProviders...)
	if err != nil {
		return nil, fmt.Errorf("httpserver: sms: %w", err)
	}
	smsSvc.SetActivity(activity.NewRecorder(deps.Queries, log))
	smsSvc.SetLogger(log)
	// E-mail one-time codes (customer portal sign-in) go through the
	// platform mailer.
	smsSvc.SetMailer(mailer)
	// Notification engine (TEC-733): rules, templates, delivery log; the
	// reservation transitions record their customer notifications (sent by
	// the worker sweep), test sends go out inline.
	notifyEngine := notifyengineusecase.New(deps.DB, deps.Queries, cfg.Auth.FrontendURL)
	notifyEngine.SetActivity(activityRec)
	notifyEngine.SetLogger(log)
	notifySMS := notifyengineadapters.NewSMS(smsSvc, deps.Queries)
	notifyEngine.Register(notifySMS, notifyengineadapters.NewEmail(mailer),
		notifyengineadapters.NewPush(notifSvc, deps.Queries, notifyengineadapters.VAPID{
			PublicKey: cfg.VAPID.PublicKey, PrivateKey: cfg.VAPID.PrivateKey, Subject: cfg.VAPID.Subject,
		}))
	integrationRunner.AddListener(notifySMS)
	paymentsSvc := paymentsusecase.New(deps.DB, deps.Queries, integrationAccountsSvc, reservationSvc,
		paymentsusecase.Config{PublicBaseURL: cfg.Auth.FrontendURL}, iyzicoAdapter)
	paymentsSvc.SetResourceEvents(resourceEvents)
	paymentsSvc.SetLogger(log)
	reservationSvc.SetPaymentLinks(paymentsSvc)
	reservationSvc.SetTransitionNotifier(notifyEngine)
	// Contracts (TEC-710): templates, rendered / hashed document versions,
	// PDFs (headless Chromium; in the worker when a queue is wired).
	contractsSvc := contractsusecase.New(deps.DB, deps.Queries, orgSettingsSvc, deps.Storage)
	contractsSvc.SetRenderer(pdfrender.NewChromium())
	if deps.Queue != nil {
		contractsSvc.SetQueue(deps.Queue)
	}
	contractsSvc.SetResourceEvents(resourceEvents)
	// Remote approval (TEC-711): signed link + SMS code through the OTP
	// service; links point at the app origin (/d/{token}).
	if err := contractsSvc.SetRemote(contractsusecase.Remote{
		SMS: smsSvc, Cipher: fieldCipher, PublicBaseURL: cfg.Auth.FrontendURL, Activity: activityRec,
	}); err != nil {
		return nil, fmt.Errorf("httpserver: contracts remote approval: %w", err)
	}
	contractsSvc.SetLogger(log)
	// Rentals (TEC-709): born from confirmed reservations with a fixed
	// plate (reservation lifecycle listener), handover / return on the
	// engine, operation tasks; documents through contracts.
	rentalsSvc := rentalusecase.New(deps.DB, deps.Queries, availabilitySvc, rentaladapters.NewReservations(reservationSvc), vehicleSvc)
	rentalsSvc.SetDocuments(contractsSvc)
	// Operation task events of rentals and inspections also refresh the
	// field task lists (board, my tasks).
	rentalsSvc.SetResourceEvents(fieldtasksusecase.MirrorPublisher{Publisher: resourceEvents})
	// Field tasks (TEC-724): delivery / collection tasks of reservations,
	// transfers on the fleet transfer flow, delivery eligibility and fees
	// of service points in quotes, assignment notifications.
	fieldTasksSvc := fieldtasksusecase.New(deps.DB, deps.Queries)
	fieldTasksSvc.SetTransfers(fleetOpsSvc)
	fieldTasksSvc.SetFinisher(fieldtasksadapters.Finisher{Rentals: rentalsSvc})
	fieldTasksSvc.SetNotifier(notifSvc)
	fieldTasksSvc.SetResourceEvents(resourceEvents)
	fieldTasksSvc.SetLogger(log)
	reservationSvc.SetDeliveryQuoter(fieldTasksSvc)
	// Channel manager (TEC-736): inventory push, booking import through
	// reservations / customers, mappings, error queue, reconciliation.
	channelsSvc, err := channelusecase.New(deps.DB, deps.Queries, fieldCipher, integrationAccountsSvc,
		channeladapters.NewBooker(reservationSvc, customerSvc), channelProviders...)
	if err != nil {
		return nil, fmt.Errorf("httpserver: channels: %w", err)
	}
	channelsSvc.SetResourceEvents(resourceEvents)
	channelsSvc.SetLogger(log)
	// Toll passes (TEC-725): provider fetch jobs on the integration runner,
	// file import fallback, matching to rentals (toll charges), review
	// queue. The finance ledger (TEC-717) plugs in with SetLedger.
	tollsSvc := tollsusecase.New(deps.DB, deps.Queries, integrationAccountsSvc, integrationRunner, integrationOps, tollProviders...)
	tollsSvc.SetResourceEvents(resourceEvents)
	tollsSvc.SetLogger(log)
	// KABİS notifications (TEC-726): recorded in the handover / return
	// transaction of rentals, sent through the integration runner.
	kabisSvc, err := kabisusecase.New(deps.DB, deps.Queries, fieldCipher, integrationRunner, integrationOps, kabisProviders...)
	if err != nil {
		return nil, fmt.Errorf("httpserver: kabis: %w", err)
	}
	kabisSvc.SetStore(deps.Storage)
	kabisSvc.SetResourceEvents(resourceEvents)
	kabisSvc.SetLogger(log)
	rentalsSvc.SetKabis(kabisSvc)
	// Inspections (TEC-712 / 713 / 714): mobile handover / return forms on
	// the rentals use cases, evidence in private storage behind signed
	// short-lived media links, signatures linked to contract approvals.
	inspectionsSvc := inspectionsusecase.New(deps.DB, deps.Queries, rentalsSvc, deps.Storage, inspectionsusecase.NewMediaSigner(cfg.JWT.AccessSecret))
	inspectionsSvc.SetContracts(contractsSvc)
	inspectionsSvc.SetResourceEvents(fieldtasksusecase.MirrorPublisher{Publisher: resourceEvents})
	inspectionsSvc.SetLogger(log)
	// Finance (TEC-717 / TEC-720): the customer / supplier ledger, cash and
	// bank accounts, expenses. Rentals accrue (rent at the handover,
	// approved charges), payments post collections and refunds, statements
	// go out over the SMS and pay-link plumbing.
	financeSvc := financeusecase.New(deps.DB, deps.Queries, deps.Storage)
	tollsSvc.SetLedger(tollsadapters.Ledger{Finance: financeSvc})
	financeSvc.SetResourceEvents(resourceEvents)
	financeSvc.SetMessaging(paymentsSvc, smsSvc)
	financeSvc.SetLogger(log)
	rentalsSvc.SetLedger(financeSvc)
	paymentsSvc.SetLedger(financeSvc)
	// Suppliers (TEC-721): the cards; their invoices and payments post
	// through finance.
	supplierSvc := supplierusecase.New(deps.DB, deps.Queries, financeSvc, searchIndexer)
	supplierSvc.SetResourceEvents(resourceEvents)
	supplierSvc.SetLogger(log)
	// Security deposits (TEC-719): online pre-auth through the
	// organization's payment account (iyzico pre-auth / post-auth / cancel).
	financeSvc.SetPreAuth(integrationAccountsSvc, cfg.Auth.FrontendURL, iyzicoAdapter)
	financeSvc.SetPayments(paymentsSvc)
	// Corporate agreements (TEC-723): the payment term goes onto the
	// customer's ledger account; tariffs are agreement-scoped rate plans.
	agreementsSvc := agreementusecase.New(deps.DB, deps.Queries, financeSvc, pricingSvc)
	agreementsSvc.SetResourceEvents(resourceEvents)
	// E-invoices / e-archive (TEC-722): drafts link the ledger's accruals,
	// go-ubltr builds and validates the UBL-TR XML, sending runs on the
	// integration runner.
	invoicesSvc, err := invoiceusecase.New(deps.DB, deps.Queries, fieldCipher, orgSettingsSvc, integrationAccountsSvc,
		integrationRunner, integrationOps, invoiceProviders...)
	if err != nil {
		return nil, fmt.Errorf("httpserver: invoices: %w", err)
	}
	invoicesSvc.SetStore(deps.Storage)
	invoicesSvc.SetPDFRenderer(pdfrender.NewChromium())
	invoicesSvc.SetResourceEvents(resourceEvents)
	invoicesSvc.SetLogger(log)
	// Customer portal (TEC-715): reference + contact + one-time code, a
	// customer session of its own, documents, pay remaining, requests.
	portalSvc := customerportalusecase.New(deps.DB, deps.Queries, smsSvc, reservationSvc)
	portalSvc.SetPayments(paymentsSvc, integrationAccountsSvc)
	portalSvc.SetDocuments(contractsSvc)
	portalSvc.SetActivity(activityRec)
	portalSvc.SetResourceEvents(resourceEvents)
	portalSvc.SetLogger(log)
	portalSvc.SetInvoices(invoicesSvc)
	// Public booking API (TEC-728): /v1/public/* for the branded site, the
	// widget and integrations, on the same quote / reservation use cases;
	// lifecycle events of its reservations become signed webhooks.
	publicAPISvc, err := publicapiusecase.New(deps.DB, deps.Queries, fieldCipher, orgSettingsSvc, reservationSvc, customerSvc, portalSvc)
	if err != nil {
		return nil, fmt.Errorf("httpserver: public api: %w", err)
	}
	publicAPISvc.SetActivity(activityRec)
	publicAPISvc.SetResourceEvents(resourceEvents)
	publicAPISvc.SetLogger(log)
	publicAPISvc.SetBaseURL(strings.TrimRight(cfg.Auth.FrontendURL, "/") + "/api")
	publicAPISvc.AllowPrivateWebhooks(!strings.EqualFold(strings.TrimSpace(cfg.App.Env), "production"))
	publicAPISvc.SetImmediateDelivery(true)
	reservationSvc.SetLifecycleListener(reservationusecase.Listeners{rentalsSvc, fieldTasksSvc, publicAPISvc, channelsSvc})
	// Rental group membership removals re-plan the commitments that relied
	// on them (or are refused); moved reservations record plan_moved.
	vehicleSvc.SetMembershipGuard(availabilitySvc)
	availabilitySvc.SetPlanMovedHandler(reservationSvc)

	// Live lists of tenant resources: org:{org_uuid}:resource:{name}, open to
	// members whose tenant role grants the resource's read permission.
	tenantLive := resourceevents.NewRegistry()
	branchesmodule.RegisterLiveResources(tenantLive)
	fleetmodule.RegisterLiveResources(tenantLive)
	organizationsmodule.RegisterLiveResources(tenantLive)
	vehiclesmodule.RegisterLiveResources(tenantLive)
	fleetopsmodule.RegisterLiveResources(tenantLive)
	pricingmodule.RegisterLiveResources(tenantLive)
	availabilitymodule.RegisterLiveResources(tenantLive)
	customersmodule.RegisterLiveResources(tenantLive)
	extrasmodule.RegisterLiveResources(tenantLive)
	campaignsmodule.RegisterLiveResources(tenantLive)
	reservationsmodule.RegisterLiveResources(tenantLive)
	paymentsmodule.RegisterLiveResources(tenantLive)
	integrationjobsmodule.RegisterLiveResources(tenantLive)
	rentalsmodule.RegisterLiveResources(tenantLive)
	fieldtasksmodule.RegisterLiveResources(tenantLive)
	inspectionsmodule.RegisterLiveResources(tenantLive)
	contractsmodule.RegisterLiveResources(tenantLive)
	customerportalmodule.RegisterLiveResources(tenantLive)
	publicapimodule.RegisterLiveResources(tenantLive)
	financemodule.RegisterLiveResources(tenantLive)
	tollsmodule.RegisterLiveResources(tenantLive)
	kabismodule.RegisterLiveResources(tenantLive)
	channelsmodule.RegisterLiveResources(tenantLive)
	agreementsmodule.RegisterLiveResources(tenantLive)
	invoicesmodule.RegisterLiveResources(tenantLive)
	suppliersmodule.RegisterLiveResources(tenantLive)
	resourceEvents.AddResolver(tenantmodule.ChannelResolver(tenantmodule.QueriesLookup(deps.Queries)))
	uc.AddChannelRules(tenantmodule.ChannelRule(tenantLive, tenantResolver))
	catalogmodule.RegisterLiveResources(liveResources)

	// Vehicle catalog (TSB kasko list): platform curation + imports, and
	// read-only lookups for organization members when tenancy is on.
	var catalogQueue catalogusecase.Enqueuer
	if deps.Queue != nil {
		catalogQueue = deps.Queue
	}
	catalogSvc := catalogusecase.New(catalogrepository.NewPostgres(deps.DB, deps.Queries), deps.Storage, catalogQueue, activityRec, log)
	catalogSvc.SetResourceEvents(resourceEvents)
	catalogmodule.RegisterRoutes(mux, cataloghandler.New(catalogSvc, activityRec), tokens, loader, tenantResolver)
	realtime.RegisterRoutes(mux, realtime.NewHandler(rtIssuer, uc), tokens, loader)

	qrGeo, err := qrlogin.NewLocator(cfg.Auth.QRGeoIPDB, cfg.Auth.QRTrustGeoHeaders)
	if err != nil {
		// Location is a hint only: keep QR sign-in up and show the IP.
		log.Warn("qrlogin_geoip_unavailable", "error", err)
	}
	var qrIssuer qrlogin.TokenIssuer
	if rtIssuer != nil {
		qrIssuer = rtIssuer
	}
	qrSvc := qrlogin.NewService(qrlogin.NewStore(deps.Redis, cfg.App.Env), uc, qrIssuer, deps.Realtime, cfg.Auth.FrontendURL, log)
	qrlogin.RegisterRoutes(mux, qrlogin.NewHandler(qrSvc, qrGeo, ratelimit.New(deps.Redis, cfg.App.Env)), tokens, loader)

	nh := notifhandler.New(notifSvc)
	nh.SetRateLimiter(ratelimit.New(deps.Redis, cfg.App.Env))
	notifmodule.RegisterRoutes(mux, nh, tokens, loader)
	notifmodule.RegisterTenantRoutes(mux, nh, tokens, loader, tenantResolver)
	notifmodule.RegisterEventHandlers(eventBus, notifSvc, log)

	// Vehicle profitability report (TEC-727): read-only over the ledger,
	// expenses and fleet tables.
	profitabilitySvc := profitabilityusecase.New(deps.Queries)
	// Reports (TEC-738): occupancy, income / expense, channels, aging and
	// the branch comparison, read-only (occupancy and branches reuse the
	// profitability engine).
	reportsSvc := reportsusecase.New(deps.Queries, profitabilitySvc)
	ioReg := ioengine.NewRegistry(
		ioadapters.NewUsers(deps.Queries),
		ioadapters.NewRoles(deps.Queries),
		ioadapters.NewNotifications(deps.Queries),
		ioadapters.NewActivity(deps.Queries),
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
		orgadapters.NewIO(orgSvc),
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
		profitabilityadapters.NewIO(profitabilitySvc),
		// Reports (TEC-738).
		reportsadapters.NewOccupancyIO(reportsSvc),
		reportsadapters.NewIncomeExpenseIO(reportsSvc),
		reportsadapters.NewChannelsIO(reportsSvc),
		reportsadapters.NewAgingIO(reportsSvc),
		reportsadapters.NewBranchesIO(reportsSvc),
	)
	exportSvc := exportusecase.New(deps.Queries, deps.Storage, ioReg, jobQueue, notifSvc, activityRec, log)
	importSvc := importusecase.New(deps.Queries, deps.Storage, ioReg, jobQueue, notifSvc, activityRec, log)
	// "Bakıma al" on the vehicles list opens maintenance blocks.
	vehicleBulk := vehicleadapters.NewBulk(vehicleSvc)
	vehicleBulk.SetMaintenance(fleetopsadapters.NewMaintenance(fleetOpsSvc))
	bulkReg := bulkengine.NewRegistry(
		bulkadapters.NewUsers(deps.Queries),
		bulkadapters.NewRoles(deps.Queries),
		branchadapters.NewBulk(branchSvc),
		fleetadapters.NewGroupsBulk(fleetSvc),
		fleetadapters.NewProductsBulk(fleetSvc),
		vehicleBulk,
		fleetopsadapters.NewOneWayBulk(fleetOpsSvc),
		extrasadapters.NewBulk(extrasSvc),
		campaignsadapters.NewBulk(campaignsSvc),
		reservationadapters.NewBulk(reservationSvc),
		fieldtasksadapters.NewBulk(fieldTasksSvc),
		orgadapters.NewBulk(orgSvc),
		customeradapters.NewBulk(customerSvc),
		financeadapters.NewMovementsBulk(financeSvc),
		financeadapters.NewExpensesBulk(financeSvc),
		supplieradapters.NewBulk(supplierSvc),
		financeadapters.NewCollectionsBulk(financeSvc),
		invoiceadapters.NewBulk(invoicesSvc),
	)
	bulkSvc := bulkusecase.New(deps.Queries, bulkReg, jobQueue, notifSvc, activityRec, cfg.Bulk, log)
	// Tenant jobs run with the requester's current membership (permissions
	// and branch scope).
	exportSvc.SetJobScopeLoader(orgSvc)
	importSvc.SetJobScopeLoader(orgSvc)
	bulkSvc.SetJobScopeLoader(orgSvc)
	logsSvc := logsusecase.New(deps.Queries)
	importSvc.SetResourceEvents(resourceEvents)
	bulkSvc.SetResourceEvents(resourceEvents)
	if s.worker != nil {
		s.worker.WithExport(exportSvc.ProcessExport).
			WithImport(importSvc.ProcessImport).
			WithBulk(bulkSvc.ProcessBulk).
			WithLogPurge(logsSvc.ApplyDueRules).
			WithMobilePush(notifSvc.SendMobilePush, notifSvc.ProcessPushReceipts).
			WithCatalogImport(catalogSvc.ProcessImport).
			WithVehicleReminders(vehicleRecordsSvc.RunReminders)
		if searchIndexer != nil {
			s.worker.WithSearch(
				searchIndexer.ProcessUpsert,
				searchIndexer.ProcessDelete,
				searchIndexer.ProcessReindex,
			)
		}
	}
	exportH := exporthandler.New(exportSvc)
	importH := importhandler.New(importSvc)
	bulkH := bulkhandler.New(bulkSvc)
	organizationsmodule.RegisterRoutes(mux, orgH, organizationsmodule.PlatformEngineHandlers{
		Export: exportH.PlatformExport(orgadapters.Resource, orgadapters.ExportQueryKeys...),
		Bulk:   bulkH.Execute(orgadapters.Resource),
		StepUp: middleware.RequireStepUp(stepUpSvc),
	}, tokens, loader)
	exportmodule.RegisterRoutes(mux, exportH, tokens, loader, stepUpSvc)
	importmodule.RegisterRoutes(mux, importH, tokens, loader)
	bulkmodule.RegisterRoutes(mux, bulkH, tokens, loader)
	// Organization-scoped engine job routes and tenant resources (mounted
	// only with TENANCY_ENABLED: a nil resolver mounts nothing).
	exportmodule.RegisterTenantRoutes(mux, exportH, tokens, loader, tenantResolver)
	importmodule.RegisterTenantRoutes(mux, importH, tokens, loader, tenantResolver)
	bulkmodule.RegisterTenantRoutes(mux, bulkH, tokens, loader, tenantResolver)
	branchH := branchhandler.New(branchSvc, activityRec)
	branchH.SetResourceEvents(resourceEvents)
	branchesmodule.RegisterRoutes(mux, branchH, branchesmodule.EngineHandlers{
		Export: exportH.TenantExport(branchesmodule.Resource, branchesmodule.ExportQueryKeys...),
		Import: importH.TenantUpload(branchesmodule.Resource),
		Sample: importH.TenantSample(branchesmodule.Resource),
		Bulk:   bulkH.TenantExecute(branchesmodule.Resource),
	}, tokens, loader, tenantResolver)
	fleetH := fleethandler.New(fleetSvc, activityRec)
	fleetH.SetResourceEvents(resourceEvents)
	fleetmodule.RegisterRoutes(mux, fleetH, fleetmodule.EngineHandlers{
		ExportSegments: exportH.TenantExport(fleetmodule.ResourceSegments, fleetmodule.SegmentExportKeys...),
		ExportGroups:   exportH.TenantExport(fleetmodule.ResourceRentalGroups, fleetmodule.GroupExportKeys...),
		ExportProducts: exportH.TenantExport(fleetmodule.ResourceRentalProducts, fleetmodule.ProductExportKeys...),
		BulkGroups:     bulkH.TenantExecute(fleetmodule.ResourceRentalGroups),
		BulkProducts:   bulkH.TenantExecute(fleetmodule.ResourceRentalProducts),
	}, tokens, loader, tenantResolver)
	vehicleH := vehiclehandler.New(vehicleSvc, activityRec)
	vehicleH.SetResourceEvents(resourceEvents)
	vehiclesmodule.RegisterRoutes(mux, vehicleH, vehiclesmodule.EngineHandlers{
		Export: exportH.TenantExport(vehiclesmodule.Resource, vehiclesmodule.ExportQueryKeys...),
		Import: importH.TenantUpload(vehiclesmodule.Resource),
		Sample: importH.TenantSample(vehiclesmodule.Resource),
		Bulk:   bulkH.TenantExecute(vehiclesmodule.Resource),
	}, tokens, loader, tenantResolver)
	vehicleRecordsH := vehiclerecordshandler.New(vehicleRecordsSvc, activityRec)
	vehicleRecordsH.SetResourceEvents(resourceEvents)
	vehiclerecordsmodule.RegisterRoutes(mux, vehicleRecordsH, vehiclerecordsmodule.EngineHandlers{
		ExportDocuments: exportH.TenantExport(
			vehiclerecordsmodule.ResourceDocuments, vehiclerecordsmodule.DocumentExportKeys...,
		),
		ExportMaintenance: exportH.TenantExport(
			vehiclerecordsmodule.ResourceMaintenance, vehiclerecordsmodule.MaintenanceExportKeys...,
		),
	}, tokens, loader, tenantResolver)
	fleetOpsH := fleetopshandler.New(fleetOpsSvc, activityRec)
	fleetOpsH.SetResourceEvents(resourceEvents)
	fleetopsmodule.RegisterRoutes(mux, fleetOpsH, fleetopsmodule.EngineHandlers{
		ExportTransfers: exportH.TenantExport(fleetopsmodule.ResourceTransfers, fleetopsmodule.TransferExportKeys...),
		ExportOneWay:    exportH.TenantExport(fleetopsmodule.ResourceOneWayRules, fleetopsmodule.OneWayExportKeys...),
		BulkOneWay:      bulkH.TenantExecute(fleetopsmodule.ResourceOneWayRules),
	}, tokens, loader, tenantResolver)
	pricingH := pricinghandler.New(pricingSvc, activityRec)
	pricingH.SetResourceEvents(resourceEvents)
	pricingmodule.RegisterRoutes(mux, pricingH, pricingmodule.EngineHandlers{
		ExportPlans: exportH.TenantExport(pricingmodule.ResourceRatePlans, pricingmodule.RatePlanExportKeys...),
		ImportPlans: importH.TenantUpload(pricingmodule.ResourceRatePlans),
		SamplePlans: importH.TenantSample(pricingmodule.ResourceRatePlans),
		ExportRules: exportH.TenantExport(pricingmodule.ResourceDateRules, pricingmodule.DateRuleExportKeys...),
	}, tokens, loader, tenantResolver)
	customerH := customerhandler.New(customerSvc, activityRec)
	customerH.SetResourceEvents(resourceEvents)
	customersmodule.RegisterRoutes(mux, customerH, customersmodule.EngineHandlers{
		Export: exportH.TenantExport(customersmodule.Resource, customersmodule.ExportQueryKeys...),
		Import: importH.TenantUpload(customersmodule.Resource),
		Sample: importH.TenantSample(customersmodule.Resource),
		Bulk:   bulkH.TenantExecute(customersmodule.Resource),
	}, tokens, loader, tenantResolver)
	availabilityH := availabilityhandler.New(availabilitySvc, activityRec)
	availabilityH.SetResourceEvents(resourceEvents)
	availabilitymodule.RegisterRoutes(mux, availabilityH, tokens, loader, tenantResolver)
	extrasH := extrashandler.New(extrasSvc, activityRec)
	extrasH.SetResourceEvents(resourceEvents)
	extrasmodule.RegisterRoutes(mux, extrasH, extrasmodule.EngineHandlers{
		Export: exportH.TenantExport(extrasmodule.Resource, extrasmodule.ExportQueryKeys...),
		Bulk:   bulkH.TenantExecute(extrasmodule.Resource),
	}, tokens, loader, tenantResolver)
	campaignsH := campaignshandler.New(campaignsSvc, activityRec)
	campaignsH.SetResourceEvents(resourceEvents)
	campaignsmodule.RegisterRoutes(mux, campaignsH, campaignsmodule.EngineHandlers{
		Export: exportH.TenantExport(campaignsmodule.Resource, campaignsmodule.ExportQueryKeys...),
		Bulk:   bulkH.TenantExecute(campaignsmodule.Resource),
	}, tokens, loader, tenantResolver)
	reservationsmodule.RegisterRoutes(mux, reservationhandler.New(reservationSvc, activityRec), reservationsmodule.EngineHandlers{
		Export: exportH.TenantExport(reservationsmodule.Resource, reservationsmodule.ExportQueryKeys...),
		Bulk:   bulkH.TenantExecute(reservationsmodule.Resource),
	}, tokens, loader, tenantResolver)
	integrationaccountsmodule.RegisterRoutes(mux, integrationaccountshandler.New(integrationAccountsSvc, activityRec), tokens, loader, tenantResolver)
	integrationjobsmodule.RegisterRoutes(mux, integrationjobshandler.New(integrationRunner, activityRec), tokens, loader, tenantResolver)
	smsmodule.RegisterRoutes(mux, smshandler.New(smsSvc, activityRec), tokens, loader, tenantResolver)
	paymentsH := paymentshandler.New(paymentsSvc, activityRec)
	paymentsH.SetRateLimiter(paymentsmodule.Limiter{L: ratelimit.New(deps.Redis, cfg.App.Env)})
	paymentsmodule.RegisterRoutes(mux, paymentsH, tokens, loader, tenantResolver)
	rentalsmodule.RegisterRoutes(mux, rentalhandler.New(rentalsSvc, activityRec), rentalsmodule.EngineHandlers{
		Export: exportH.TenantExport(rentalsmodule.Resource, rentalsmodule.ExportQueryKeys...),
	}, tokens, loader, tenantResolver)
	kabismodule.RegisterRoutes(mux, kabishandler.New(kabisSvc, activityRec), tokens, loader, tenantResolver)
	channelsmodule.RegisterRoutes(mux, channelhandler.New(channelsSvc, activityRec), tokens, loader, tenantResolver)
	agreementsmodule.RegisterRoutes(mux, agreementhandler.New(agreementsSvc, activityRec), agreementsmodule.EngineHandlers{
		Export: exportH.TenantExport(agreementsmodule.Resource, agreementsmodule.ExportQueryKeys...),
	}, tokens, loader, tenantResolver)
	tollsmodule.RegisterRoutes(mux, tollshandler.New(tollsSvc, activityRec), tollsmodule.EngineHandlers{
		Export: exportH.TenantExport(tollsmodule.Resource, tollsmodule.ExportQueryKeys...),
		Import: importH.TenantUpload(tollsmodule.Resource),
		Sample: importH.TenantSample(tollsmodule.Resource),
	}, tokens, loader, tenantResolver)
	fieldtasksmodule.RegisterRoutes(mux, fieldtaskshandler.New(fieldTasksSvc, activityRec), fieldtasksmodule.EngineHandlers{
		Bulk: bulkH.TenantExecute(fieldtasksmodule.Resource),
	}, tokens, loader, tenantResolver)
	// Operations overview (TEC-716): the tenant home page and the sidebar
	// badge counts, read-only over the modules above.
	overviewmodule.RegisterRoutes(mux, overviewhandler.New(overviewusecase.New(deps.Queries)), tokens, loader, tenantResolver)
	profitabilitymodule.RegisterRoutes(mux, profitabilityhandler.New(profitabilitySvc),
		exportH.TenantExport(profitabilitymodule.Resource, profitabilitymodule.ExportQueryKeys...), tokens, loader, tenantResolver)
	reportsmodule.RegisterRoutes(mux, reportshandler.New(reportsSvc), exportH.TenantExport, tokens, loader, tenantResolver)
	// Platform usage and connection health (TEC-739). Modules with their
	// own sync / delivery health register a healthcheck.Source on
	// healthRegistry here.
	healthRegistry := platformhealthmodule.NewRegistry(deps.Queries)
	platformhealthmodule.RegisterRoutes(mux, platformhealthhandler.New(platformhealthusecase.New(deps.Queries, healthRegistry)), tokens, loader)
	contractsH := contractshandler.New(contractsSvc, activityRec)
	contractsH.SetRateLimiter(contractsmodule.Limiter{L: ratelimit.New(deps.Redis, cfg.App.Env)})
	contractsmodule.RegisterRoutes(mux, contractsH, tokens, loader, tenantResolver)
	inspectionsmodule.RegisterRoutes(mux, inspectionshandler.New(inspectionsSvc, activityRec), tokens, loader, tenantResolver)
	financemodule.RegisterRoutes(mux, financehandler.New(financeSvc, activityRec), financemodule.EngineHandlers{
		Accounts: financemodule.ResourceEngines{
			Export: exportH.TenantExport(financemodule.ResourceAccounts, financemodule.AccountQueryKeys...),
		},
		Entries: financemodule.ResourceEngines{
			Export: exportH.TenantExport(financemodule.ResourceEntries, financemodule.EntryQueryKeys...),
		},
		CashAccounts: financemodule.ResourceEngines{
			Export: exportH.TenantExport(financemodule.ResourceCashAccounts, financemodule.CashQueryKeys...),
			Import: importH.TenantUpload(financemodule.ResourceCashAccounts),
			Sample: importH.TenantSample(financemodule.ResourceCashAccounts),
		},
		Movements: financemodule.ResourceEngines{
			Export: exportH.TenantExport(financemodule.ResourceMovements, financemodule.MovementQueryKeys...),
			Import: importH.TenantUpload(financemodule.ResourceMovements),
			Sample: importH.TenantSample(financemodule.ResourceMovements),
			Bulk:   bulkH.TenantExecute(financemodule.ResourceMovements),
		},
		Expenses: financemodule.ResourceEngines{
			Export: exportH.TenantExport(financemodule.ResourceExpenses, financemodule.ExpenseQueryKeys...),
			Import: importH.TenantUpload(financemodule.ResourceExpenses),
			Sample: importH.TenantSample(financemodule.ResourceExpenses),
			Bulk:   bulkH.TenantExecute(financemodule.ResourceExpenses),
		},
		Collections: financemodule.ResourceEngines{
			Export: exportH.TenantExport(financemodule.ResourceCollections, financemodule.CollectionQueryKeys...),
			Bulk:   bulkH.TenantExecute(financemodule.ResourceCollections),
		},
		Deposits: financemodule.ResourceEngines{
			Export: exportH.TenantExport(financemodule.ResourceDeposits, financemodule.DepositQueryKeys...),
		},
		SupplierInvoices: financemodule.ResourceEngines{
			Export: exportH.TenantExport(financemodule.ResourceSupplierInvoices, financemodule.SupplierInvoiceQueryKeys...),
		},
		SupplierPayments: financemodule.ResourceEngines{
			Export: exportH.TenantExport(financemodule.ResourceSupplierPayments, financemodule.SupplierPaymentQueryKeys...),
		},
	}, tokens, loader, tenantResolver)
	suppliersmodule.RegisterRoutes(mux, supplierhandler.New(supplierSvc, activityRec), suppliersmodule.EngineHandlers{
		Export: exportH.TenantExport(suppliersmodule.Resource, suppliersmodule.QueryKeys...),
		Import: importH.TenantUpload(suppliersmodule.Resource),
		Sample: importH.TenantSample(suppliersmodule.Resource),
		Bulk:   bulkH.TenantExecute(suppliersmodule.Resource),
	}, tokens, loader, tenantResolver)
	invoicesmodule.RegisterRoutes(mux, invoicehandler.New(invoicesSvc, activityRec), invoicesmodule.Engines{
		Export: exportH.TenantExport(invoicesmodule.Resource, invoicesmodule.ExportQueryKeys...),
		Bulk:   bulkH.TenantExecute(invoicesmodule.Resource),
	}, tokens, loader, tenantResolver)
	portalH := customerportalhandler.New(portalSvc, activityRec)
	portalH.SetRateLimiter(customerportalmodule.Limiter{L: ratelimit.New(deps.Redis, cfg.App.Env)})
	customerportalmodule.RegisterRoutes(mux, portalH, tokens, loader, tenantResolver)
	publicapimodule.RegisterRoutes(mux,
		publicapihandler.NewPublic(publicAPISvc, ratelimit.New(deps.Redis, cfg.App.Env)),
		publicapihandler.NewPanel(publicAPISvc, activityRec), tokens, loader, tenantResolver)
	notifyH := notifyenginehandler.New(notifyEngine, cfg.VAPID.PublicKey)
	notifyenginemodule.RegisterRoutes(mux, notifyH, tokens, loader, tenantResolver)
	if tenantResolver != nil {
		notifyenginemodule.RegisterCustomerRoutes(mux, notifyH, portalH.RequireSession)
	}
	settingsmodule.RegisterRoutes(mux, settingshandler.New(
		settingsusecase.New(deps.Queries),
		deps.Storage,
		settingsusecase.NewSystemService(cfg, deps.DB, deps.Redis, deps.Storage, searchClient),
	), tokens, loader)
	accessmodule.RegisterRoutes(mux, accesshandler.New(stepUpSvc, activityRec), tokens, loader)
	authsettingsmodule.RegisterRoutes(mux, authsettingshandler.New(authSettingsSvc, activityRec), tokens, loader)
	githubmodule.RegisterRoutes(mux, githubhandler.New(githubSvc, activityRec), tokens, loader)
	oauthprovidermodule.RegisterRoutes(mux, oauthproviderhandler.New(oauthProvSvc, activityRec), tokens, loader)
	activitymodule.RegisterRoutes(mux, activityhandler.New(activityusecase.New(deps.Queries)), tokens, loader)
	logsmodule.RegisterRoutes(mux, logshandler.New(logsSvc), tokens, loader)
	searchSvc := searchusecase.New(searchClient, searchReg, log)
	searchH := searchhandler.New(searchSvc)
	searchmodule.RegisterRoutes(mux, searchH, tokens, loader)
	searchmodule.RegisterTenantRoutes(mux, searchH, tokens, loader, tenantResolver)
	storagemodule.RegisterRoutes(
		mux,
		storagehandler.New(storageusecase.New(deps.Storage, deps.Queries, log)),
		tokens,
		loader,
	)

	outboxStore := outbox.NewStore(deps.DB, deps.Queries)
	outboxPub := outbox.NewPublisher(outboxStore, eventBus, log)
	s.outboxPub = outboxPub

	s.http = &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      middleware.RequestID(middleware.ServerErrors(log)(mux)),
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}
	return s, nil
}

// Start listens until the server stops. Optionally starts an in-process queue worker.
func (s *Server) Start() error {
	if s.outboxPub != nil {
		s.outboxStop = s.outboxPub.StartRun(context.Background())
	}
	if s.worker != nil {
		go func() {
			if err := s.worker.Start(); err != nil {
				s.log.Error("queue_worker_failed", "error", err)
			}
		}()
	}
	if s.searchIndexer != nil {
		go s.searchIndexer.Bootstrap(context.Background())
	}
	s.log.Info("http_listen", "addr", s.cfg.HTTP.Addr, "env", s.cfg.App.Env)
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("httpserver: listen: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the HTTP server, queue worker, and queue client.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.outboxStop != nil {
		s.outboxStop()
	}
	if s.worker != nil {
		s.worker.Shutdown()
	}
	if s.queueClient != nil {
		_ = s.queueClient.Close()
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) handleAppConfig(w http.ResponseWriter, r *http.Request) {
	settingsSvc := settingsusecase.New(s.queries)
	letterhead, _ := settingsSvc.PublicLetterhead(r.Context())

	registrationEnabled := false
	passwordLogin := true
	passwordRegister := false
	passkeyLogin := true
	if s.authSettings != nil {
		if p, err := s.authSettings.Policy(r.Context()); err == nil {
			registrationEnabled = p.RegistrationEnabled
			passwordLogin = p.PasswordLoginEnabled
			passwordRegister = p.RegistrationEnabled && p.PasswordRegisterEnabled
			passkeyLogin = p.PasskeyLoginEnabled
		}
	}

	githubLogin, githubRegister := false, false
	if s.githubSvc != nil {
		githubLogin, _ = s.githubSvc.IsAuthEnabled(r.Context())
		reg, _ := s.githubSvc.IsRegisterEnabled(r.Context())
		githubRegister = registrationEnabled && reg
	}

	method := func(login, register bool) map[string]bool {
		return map[string]bool{"login": login, "register": register}
	}
	oauthMethod := func(provider string) map[string]bool {
		login, register := false, false
		if s.oauthProvSvc != nil {
			login, _ = s.oauthProvSvc.IsLoginEnabled(r.Context(), provider)
			reg, _ := s.oauthProvSvc.IsRegisterEnabled(r.Context(), provider)
			register = registrationEnabled && reg
		}
		return method(login, register)
	}

	responseJSON := map[string]any{
		"mode":                 "platform",
		"name":                 s.cfg.App.Name,
		"vapid_configured":     s.cfg.VAPID.PublicKey != "" && s.cfg.VAPID.PrivateKey != "",
		"github_auth_enabled":  githubLogin, // legacy
		"registration_enabled": registrationEnabled,
		"auth_methods": map[string]any{
			"password": method(passwordLogin, passwordRegister),
			"passkey":  map[string]bool{"login": passkeyLogin},
			"github":   method(githubLogin, githubRegister),
			"google":   oauthMethod("google"),
			"facebook": oauthMethod("facebook"),
			"apple":    oauthMethod("apple"),
		},
		"letterhead": letterhead,
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    responseJSON,
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{
		"postgres": "ok",
		"redis":    "ok",
	}
	ready := true

	if err := database.Ping(ctx, s.db); err != nil {
		checks["postgres"] = "down"
		ready = false
	}
	if err := cache.Ping(ctx, s.redis); err != nil {
		checks["redis"] = "down"
		ready = false
	}

	if s.storage != nil {
		if err := s.storage.Ping(ctx); err != nil {
			s.log.Warn("readyz_storage_soft_check", "error", err)
		}
	}
	if s.searchClient != nil && s.searchClient.Enabled() {
		if err := s.searchClient.Ping(ctx); err != nil {
			checks["meilisearch"] = "down"
			s.log.Warn("readyz_meilisearch_soft_check", "error", err)
		} else {
			checks["meilisearch"] = "ok"
		}
	}

	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}

	writeJSON(w, status, map[string]any{
		"status": state,
		"checks": checks,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
