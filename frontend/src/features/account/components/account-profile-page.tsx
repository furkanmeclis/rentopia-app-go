"use client";

import { Suspense, type ReactNode } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  Bell,
  Fingerprint,
  KeyRound,
  Link2,
  Monitor,
  ShieldCheck,
  TriangleAlert,
  UserRound,
  type LucideIcon,
} from "lucide-react";
import { toast } from "sonner";

import { PageHeader } from "@/components/layout";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/common/card";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { ChangePasswordForm } from "@/features/account/components/change-password-form";
import { DeleteAccountDialog } from "@/features/account/components/delete-account-dialog";
import { OAuthIdentityManager } from "@/features/account/components/github-identity-manager";
import { NotificationPreferencesForm } from "@/features/account/components/notification-preferences-form";
import { PasskeyManager } from "@/features/account/components/passkey-manager";
import { SessionManager } from "@/features/account/components/session-manager";
import { TotpManager } from "@/features/account/components/totp-manager";
import { ProfileEditForm } from "@/features/account/components/profile-edit-form";
import { ProfilePermissionsList } from "@/features/account/components/profile-permissions-list";
import { isApiError } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";
import { fetchAppPublicConfig } from "@/services/app-config.service";
import { cn } from "@/lib/utils";

type AccountShell = "platform" | "account";

function homeHref(shell: AccountShell) {
  return shell === "platform" ? routes.platform.home : routes.account.home;
}

type ProfileSection =
  | "general"
  | "security"
  | "connections"
  | "sessions"
  | "notifications"
  | "permissions"
  | "danger";

const SECTIONS: {
  id: ProfileSection;
  icon: LucideIcon;
  labelKey: string;
  hintKey: string;
}[] = [
  {
    id: "general",
    icon: UserRound,
    labelKey: "auth.profile.nav_general",
    hintKey: "auth.profile.nav_general_hint",
  },
  {
    id: "security",
    icon: ShieldCheck,
    labelKey: "auth.profile.nav_security",
    hintKey: "auth.profile.nav_security_hint",
  },
  {
    id: "connections",
    icon: Link2,
    labelKey: "auth.profile.nav_connections",
    hintKey: "auth.oauth.profile_hint",
  },
  {
    id: "sessions",
    icon: Monitor,
    labelKey: "auth.profile.nav_sessions",
    hintKey: "auth.sessions.description",
  },
  {
    id: "notifications",
    icon: Bell,
    labelKey: "auth.profile.nav_notifications",
    hintKey: "auth.preferences.description",
  },
  {
    id: "permissions",
    icon: KeyRound,
    labelKey: "auth.profile.nav_permissions",
    hintKey: "auth.profile.permissions_hint",
  },
  {
    id: "danger",
    icon: TriangleAlert,
    labelKey: "auth.profile.nav_danger",
    hintKey: "auth.delete_account.description",
  },
];

function isSection(value: string | null): value is ProfileSection {
  return SECTIONS.some((section) => section.id === value);
}

/** Profile with a section sub-nav; the open section lives in `?tab=`. */
export function AccountProfilePage({ shell }: { shell: AccountShell }) {
  return (
    <Suspense fallback={null}>
      <AccountProfileContent shell={shell} />
    </Suspense>
  );
}

function AccountProfileContent({ shell }: { shell: AccountShell }) {
  const { t } = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const raw = searchParams.get("tab");
  const active: ProfileSection = isSection(raw) ? raw : "general";
  const current = SECTIONS.find((section) => section.id === active)!;

  const select = (id: ProfileSection) => {
    const params = new URLSearchParams(searchParams.toString());
    if (id === "general") params.delete("tab");
    else params.set("tab", id);
    const query = params.toString();
    router.replace(query ? `${pathname}?${query}` : pathname, {
      scroll: false,
    });
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("auth.profile.title")}
        description={t("auth.profile.description")}
        breadcrumbs={[
          { label: t("layout.breadcrumb_home"), href: homeHref(shell) },
          { label: t("auth.profile") },
        ]}
      />

      <ProfileIdentity />

      <div className="grid gap-6 lg:grid-cols-[14rem_minmax(0,1fr)] lg:items-start">
        <nav
          aria-label={t("auth.profile.nav_label")}
          className="-mx-4 overflow-x-auto px-4 lg:sticky lg:top-20 lg:mx-0 lg:overflow-visible lg:px-0"
        >
          <ul className="flex gap-1 lg:flex-col">
            {SECTIONS.map(({ id, icon: Icon, labelKey }) => {
              const selected = id === active;
              return (
                <li key={id} className="shrink-0">
                  <Button
                    type="button"
                    variant="ghost"
                    aria-current={selected ? "page" : undefined}
                    onClick={() => select(id)}
                    className={cn(
                      "text-muted-foreground h-9 w-full justify-start gap-2.5 px-3 font-medium",
                      selected &&
                        "bg-primary/10 text-primary hover:bg-primary/15 hover:text-primary",
                      id === "danger" && !selected && "hover:text-destructive",
                      id === "danger" &&
                        selected &&
                        "bg-destructive/10 text-destructive hover:bg-destructive/15 hover:text-destructive",
                    )}
                  >
                    <Icon className="size-4" aria-hidden />
                    {t(labelKey)}
                  </Button>
                </li>
              );
            })}
          </ul>
        </nav>

        <section className="min-w-0 space-y-4">
          <div className="space-y-1">
            <h2 className="text-lg font-semibold tracking-tight">
              {t(current.labelKey)}
            </h2>
            <CardHint>{t(current.hintKey)}</CardHint>
          </div>
          <SectionBody section={active} />
        </section>
      </div>
    </div>
  );
}

function ProfileIdentity() {
  const { t } = useLocale();
  const { user, hydrateProfile } = useAuth();

  const roles = user?.roles?.length
    ? user.roles
    : user?.isSuperAdmin
      ? ["super_admin"]
      : [];
  const initials =
    (user?.fullName ?? "")
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part[0]?.toLocaleUpperCase("tr"))
      .join("") || "?";

  const requestVerify = async () => {
    try {
      await authService.requestEmailVerify(user?.email);
      toast.success(t("auth.verify.request_success"));
    } catch (error) {
      if (isApiError(error)) {
        toast.error(error.message || t("common.error_generic"));
      } else {
        toast.error(t("common.error_generic"));
      }
    } finally {
      await hydrateProfile();
    }
  };

  return (
    <Card className="shadow-none">
      <CardContent className="flex flex-col gap-4 sm:flex-row sm:items-center">
        <Avatar className="size-16 text-lg">
          <AvatarFallback className="bg-primary text-primary-foreground font-semibold">
            {initials}
          </AvatarFallback>
        </Avatar>
        <div className="min-w-0 flex-1 space-y-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <p className="truncate text-lg font-semibold">
              {user?.fullName || "—"}
            </p>
            {roles.map((role) => (
              <Badge key={role} variant="secondary">
                {role}
              </Badge>
            ))}
          </div>
          <div className="text-muted-foreground flex flex-wrap items-center gap-2 text-sm">
            <span className="truncate">{user?.email || "—"}</span>
            {user?.emailVerified ? (
              <Badge variant="success">{t("auth.profile.verified")}</Badge>
            ) : (
              <>
                <Badge variant="warning">{t("auth.profile.unverified")}</Badge>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={requestVerify}
                >
                  {t("auth.verify.request")}
                </Button>
              </>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function SectionBody({ section }: { section: ProfileSection }) {
  const { t, locale } = useLocale();
  const { user } = useAuth();
  const { data: appConfig } = useQuery({
    queryKey: ["app", "config", locale],
    queryFn: () => fetchAppPublicConfig(locale),
    enabled: section === "connections",
  });

  switch (section) {
    case "general":
      return (
        <>
          <Card className="shadow-none">
            <CardHeader>
              <CardTitle>{t("auth.profile.edit_title")}</CardTitle>
              <CardHint>{t("auth.profile.edit_hint")}</CardHint>
            </CardHeader>
            <CardContent>
              <ProfileEditForm />
            </CardContent>
          </Card>
          <Card className="shadow-none">
            <CardContent>
              <dl className="grid gap-4 sm:grid-cols-2">
                <Fact
                  label={t("auth.profile.status")}
                  value={
                    user?.status ? (
                      <Badge variant="secondary">
                        {user.status === "active"
                          ? t("auth.profile.status_active")
                          : user.status}
                      </Badge>
                    ) : (
                      "—"
                    )
                  }
                />
                <Fact
                  label={t("auth.profile.uuid")}
                  value={
                    <span className="font-mono text-xs">
                      {user?.uuid ?? "—"}
                    </span>
                  }
                />
              </dl>
            </CardContent>
          </Card>
        </>
      );
    case "security":
      return (
        <>
          <Card className="shadow-none">
            <CardHeader>
              <CardTitle>{t("auth.password.title")}</CardTitle>
              <CardHint>{t("auth.profile.change_password_body")}</CardHint>
            </CardHeader>
            <CardContent>
              <ChangePasswordForm />
            </CardContent>
          </Card>
          <Card className="shadow-none">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Fingerprint
                  className="text-muted-foreground size-5"
                  aria-hidden
                />
                {t("auth.passkey.title")}
              </CardTitle>
              <CardHint>{t("auth.passkey.description")}</CardHint>
            </CardHeader>
            <CardContent>
              <PasskeyManager />
            </CardContent>
          </Card>
          <Card className="shadow-none">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <ShieldCheck
                  className="text-muted-foreground size-5"
                  aria-hidden
                />
                {t("auth.totp.title")}
              </CardTitle>
              <CardHint>{t("auth.totp.description")}</CardHint>
            </CardHeader>
            <CardContent>
              <TotpManager />
            </CardContent>
          </Card>
        </>
      );
    case "connections":
      return (
        <Card className="shadow-none">
          <CardContent>
            <OAuthIdentityManager
              providers={[
                {
                  id: "github",
                  enabled: Boolean(appConfig?.auth_methods.github.login),
                },
                {
                  id: "google",
                  enabled: Boolean(appConfig?.auth_methods.google.login),
                },
                {
                  id: "facebook",
                  enabled: Boolean(appConfig?.auth_methods.facebook.login),
                },
                {
                  id: "apple",
                  enabled: Boolean(appConfig?.auth_methods.apple.login),
                },
              ]}
            />
          </CardContent>
        </Card>
      );
    case "sessions":
      return (
        <Card className="shadow-none">
          <CardContent>
            <SessionManager />
          </CardContent>
        </Card>
      );
    case "notifications":
      return (
        <Card className="shadow-none">
          <CardContent>
            <NotificationPreferencesForm />
          </CardContent>
        </Card>
      );
    case "permissions":
      return (
        <Card className="shadow-none">
          <CardContent>
            <ProfilePermissionsList granted={user?.permissions} />
          </CardContent>
        </Card>
      );
    case "danger":
      return (
        <Card className="border-destructive/40 shadow-none">
          <CardHeader>
            <CardTitle>{t("auth.delete_account.title")}</CardTitle>
          </CardHeader>
          <CardContent>
            <DeleteAccountDialog />
          </CardContent>
        </Card>
      );
  }
}

function CardHint({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <p className={cn("text-muted-foreground text-sm", className)}>{children}</p>
  );
}

function Fact({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="min-w-0 space-y-1">
      <dt className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
        {label}
      </dt>
      <dd className="text-sm font-medium break-all">{value}</dd>
    </div>
  );
}
