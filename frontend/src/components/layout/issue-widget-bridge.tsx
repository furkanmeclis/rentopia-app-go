"use client";

import { useEffect } from "react";
import { usePathname } from "next/navigation";

import { callIssueWidget } from "@/lib/issue-widget";
import { useAuthStore } from "@/lib/auth/session-store";

/** Shell of the current page, from the first path segment. */
function shellOf(pathname: string): { shell: string; tenant: string | null } {
  const [first, second] = pathname.split("/").filter(Boolean);
  if (first === "t" && second) return { shell: "tenant", tenant: second };
  if (first === "platform") return { shell: "platform", tenant: null };
  if (first === "account") return { shell: "account", tenant: null };
  return { shell: "public", tenant: null };
}

/**
 * Hands the signed-in staff user and the current shell / organization to
 * the feedback widget. Renders nothing; skipped while no one is signed in.
 */
export function IssueWidgetBridge() {
  const user = useAuthStore((s) => s.user);
  const pathname = usePathname() ?? "/";

  const userId = user?.uuid;
  const userName = user?.fullName;
  const userEmail = user?.email;
  useEffect(() => {
    if (!userId) return;
    callIssueWidget([
      "setUser",
      { id: userId, name: userName || undefined, email: userEmail },
    ]);
  }, [userId, userName, userEmail]);

  const { shell, tenant } = shellOf(pathname);
  const tenantName =
    tenant && user
      ? (user.organizations.find((o) => o.slug === tenant)?.name ?? null)
      : null;
  useEffect(() => {
    callIssueWidget(["setContext", { shell, tenant, tenant_name: tenantName }]);
  }, [shell, tenant, tenantName]);

  return null;
}
