SHELL := /bin/bash

ENV_FILE ?= $(firstword $(wildcard .env))
PROD_ENV_FILE ?= .env.server

define load_env
if [ -f "$(ENV_FILE)" ]; then \
	set -a; \
	while IFS= read -r line || [ -n "$$line" ]; do \
		case "$$line" in \#*|'') continue ;; esac; \
		if [[ "$$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$$ ]]; then \
			export "$${BASH_REMATCH[1]}=$${BASH_REMATCH[2]}"; \
		fi; \
	done < "$(ENV_FILE)"; \
	set +a; \
fi;
endef

FRONTEND_PORT ?= 3000

.PHONY: infra infra-down infra-logs infra-ps migrate-up backend-dev frontend-dev site-dev local-dev \
	free-dev-ports prod-env prod-env-upgrade prod-env-check prod-sync prod-super-admin prod-config prod-up prod-down \
	prod-backup prod-backup-status tools create-super-admin search-reindex check-i18n \
	messaging-reconcile prod-messaging-reconcile integration-jobs-reconcile prod-integration-jobs-reconcile \
	acceptance acceptance-backend acceptance-e2e acceptance-stack-up acceptance-stack-down

infra:
	@if [ -n "$(ENV_FILE)" ]; then \
		docker compose --env-file "$(ENV_FILE)" -f compose.local.yml up -d; \
	else \
		docker compose -f compose.local.yml up -d; \
	fi

infra-down:
	@if [ -n "$(ENV_FILE)" ]; then \
		docker compose --env-file "$(ENV_FILE)" -f compose.local.yml down; \
	else \
		docker compose -f compose.local.yml down; \
	fi

infra-logs:
	docker compose -f compose.local.yml logs -f

infra-ps:
	docker compose -f compose.local.yml ps

migrate-up:
	@$(load_env) $(MAKE) -C backend migrate-up

backend-dev:
	@$(load_env) $(MAKE) -C backend dev

frontend-dev:
	@$(load_env) \
	cd frontend && \
	if [ ! -d node_modules ]; then pnpm install; fi && \
	pnpm dev

# Branded site app (TEC-729) on :3100; open http://{subdomain}.localhost:3100.
site-dev:
	@$(load_env) \
	cd site && \
	if [ ! -d node_modules ]; then pnpm install; fi && \
	pnpm dev

# Host ports bound by air + Next.js. Leaves Docker-mapped infra ports alone.
free-dev-ports:
	@$(load_env) \
	free_listen_port() { \
		local port="$$1"; \
		if ! command -v lsof >/dev/null 2>&1; then \
			echo "lsof not found; skip :$$port"; \
			return 0; \
		fi; \
		local attempt pids pid comm ppid pcomm sig; \
		for attempt in 1 2; do \
			pids=$$(lsof -nP -iTCP:"$$port" -sTCP:LISTEN -t 2>/dev/null | sort -u); \
			if [ -z "$$pids" ]; then \
				if [ "$$attempt" -eq 1 ]; then echo ":$$port is free"; fi; \
				break; \
			fi; \
			for pid in $$pids; do \
				comm=$$(ps -o comm= -p "$$pid" 2>/dev/null | tr -d '[:space:]'); \
				case "$$comm" in \
					*docker*|*com.docker*|dockerd) echo "skipping docker on :$$port (pid $$pid)"; continue ;; \
				esac; \
				ppid=$$(ps -o ppid= -p "$$pid" 2>/dev/null | tr -d '[:space:]'); \
				pcomm=$$(ps -o comm= -p "$$ppid" 2>/dev/null | tr -d '[:space:]'); \
				sig="-TERM"; \
				if [ "$$attempt" -eq 2 ]; then sig="-KILL"; fi; \
				echo "freeing :$$port pid $$pid ($$comm) $$sig"; \
				case "$$pcomm" in \
					*air*|*next*|*node*|*pnpm*|*npm*) kill "$$sig" "$$ppid" 2>/dev/null || true ;; \
				esac; \
				kill "$$sig" "$$pid" 2>/dev/null || true; \
			done; \
			sleep 0.4; \
		done; \
	}; \
	http_addr="$${APP_HTTP_ADDR:-:8080}"; \
	backend_port="$${http_addr##*:}"; \
	frontend_port="$${PORT:-$(FRONTEND_PORT)}"; \
	echo "Checking host ports $$backend_port (api) and $$frontend_port (frontend)..."; \
	free_listen_port "$$backend_port"; \
	free_listen_port "$$frontend_port"

# Infra in Docker; backend (air) + frontend (pnpm) on the host. No app image builds.
local-dev: infra
	@echo "Waiting for postgres..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
		docker compose -f compose.local.yml exec -T postgres pg_isready -U $${DB_USER:-app} -d $${DB_NAME:-app} >/dev/null 2>&1 && break; \
		sleep 1; \
	done
	@$(load_env) $(MAKE) -C backend migrate-up || true
	@$(MAKE) free-dev-ports
	@echo "Starting backend (air) + frontend (pnpm) — Ctrl+C stops both"
	@trap 'kill 0' INT TERM; \
	$(MAKE) backend-dev & \
	$(MAKE) frontend-dev & \
	wait

prod-env:
	scripts/prod-env.sh init -o $(PROD_ENV_FILE)

prod-env-upgrade:
	scripts/prod-env.sh upgrade $(PROD_ENV_FILE)

prod-env-check:
	scripts/prod-env.sh check $(PROD_ENV_FILE)

prod-sync:
	scripts/server-sync.sh

prod-super-admin:
	docker compose --env-file $(PROD_ENV_FILE) -f compose.prod.yml exec backend /app/create-super-admin \
		-email "$(SA_EMAIL)" -password "$(SA_PASSWORD)" -name "$(or $(SA_NAME),Platform)" -surname "$(or $(SA_SURNAME),Admin)"

prod-config:
	docker compose --env-file $(PROD_ENV_FILE) -f compose.prod.yml config

prod-up:
	docker compose --env-file $(PROD_ENV_FILE) -f compose.prod.yml up -d --build

prod-down:
	docker compose --env-file $(PROD_ENV_FILE) -f compose.prod.yml down

# Encrypted backup set now (Postgres + Redis + bucket); see docs/operations/backup-restore.md.
# Restores are deliberate: run scripts/restore.sh by hand.
prod-backup:
	scripts/backup.sh --env-file $(PROD_ENV_FILE)

prod-backup-status:
	scripts/backup.sh status --env-file $(PROD_ENV_FILE)

tools:
	@echo "docker:  $$(docker version --format '{{.Server.Version}}' 2>&1)"
	@echo "compose: $$(docker compose version 2>&1)"
	@echo "env:     $(ENV_FILE)"
	@$(MAKE) -C backend tools

# Bootstrap platform super_admin (loads root .env). Override: SA_EMAIL SA_PASSWORD SA_NAME SA_SURNAME
SA_EMAIL    ?= admin@example.com
SA_PASSWORD ?= Password1
SA_NAME     ?= Platform
SA_SURNAME  ?= Admin

create-super-admin:
	@$(load_env) $(MAKE) -C backend create-super-admin \
		SA_EMAIL="$(SA_EMAIL)" \
		SA_PASSWORD="$(SA_PASSWORD)" \
		SA_NAME="$(SA_NAME)" \
		SA_SURNAME="$(SA_SURNAME)"

search-reindex:
	@$(load_env) $(MAKE) -C backend search-reindex

# Outbound messages waiting in restored data (docs/operations/backup-restore.md
# "Mesaj mutabakatı"): ARGS="list --before <set time>" / ARGS="mark ... --yes".
messaging-reconcile:
	@$(load_env) $(MAKE) -C backend messaging-reconcile ARGS='$(ARGS)'

prod-messaging-reconcile:
	docker compose --env-file $(PROD_ENV_FILE) -f compose.prod.yml run --rm --no-deps \
		--entrypoint /app/messaging-reconcile backend $(ARGS)

# Integration jobs done after the backup (docs/operations/backup-restore.md
# RT-A32 (c)): ARGS="--before <set time>" lists, add "--apply --note ..." to mark.
integration-jobs-reconcile:
	@$(load_env) $(MAKE) -C backend integration-jobs-reconcile ARGS='$(ARGS)'

prod-integration-jobs-reconcile:
	docker compose --env-file $(PROD_ENV_FILE) -f compose.prod.yml run --rm --no-deps \
		--entrypoint /app/integration-jobs-reconcile backend $(ARGS)

check-i18n:
	node scripts/check-i18n.mjs

# --- Acceptance suite RT-A01…A32 (TEC-740, docs/acceptance/beta.md) ---------
# Go: every TestRT_A* / *RTA* test plus the older tests that prove a scenario
# under another name (mapping in docs/acceptance/beta.md). Needs a migrated
# TEST_DATABASE_URL (same as CI's backend job); without it the DB tests skip.
ACCEPTANCE_GO_RUN ?= RT_A|RTA|^(TestPricingEndToEnd|TestPreviewExtrasAndCoupons|TestOneWayReturnMovesTheVehicle|TestBlockInTheMiddle|TestModelGuaranteeNotSoldOnOtherModel|TestBranchScope|TestVerificationMismatchIsRefused|TestLatePaymentReinstatesWhenCapacityFree|TestWebhookPostingIsIdempotent|TestRunnerUncertainChecksStatusWithoutDoubleSubmit|TestRunnerResubmitsOnlyWhenConfirmedNotDone|TestBuildAndValidateTemelAndEArsiv|TestIdempotentSendCreatesOneInvoice|TestSendFailureKeepsFinance|TestDoubleInvoicingRefused|TestUncertainSendChecksStatusFirst|TestCatalogAndSameAnswerEverywhere|TestPlatformPlanRoutes|TestPlanFeatureGate|TestRequireFeature|TestCustomerLinksAreNeverGated|TestPublicSiteOutsidePlanIsNotFound|TestFallbackAfterUndeliveredReport|TestFallbackAfterRejectionAndDelivered)$$

acceptance-backend:
	@if [ -z "$$TEST_DATABASE_URL" ]; then echo "TEST_DATABASE_URL is not set: the database scenarios would skip (see docs/acceptance/beta.md)"; exit 1; fi
	cd backend && go test ./... -count=1 -v -run '$(ACCEPTANCE_GO_RUN)'

# Playwright stack: Postgres / Redis / SeaweedFS containers, API :18080,
# panel :13000 (next dev), site :13100 (frontend/e2e/stack/env.sh).
acceptance-stack-up:
	frontend/e2e/stack/up.sh

acceptance-stack-down:
	frontend/e2e/stack/down.sh

acceptance-e2e:
	cd frontend && pnpm e2e

# Everything: Go scenarios, then the stack, the Playwright scenarios and the
# stack teardown (also on failure).
acceptance: acceptance-backend
	@frontend/e2e/stack/up.sh && status=0 && (cd frontend && pnpm e2e) || status=$$?; \
	frontend/e2e/stack/down.sh; exit $$status
