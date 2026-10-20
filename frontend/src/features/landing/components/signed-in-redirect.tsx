"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { defaultHomeForUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";

/**
 * Safety net for a client-side visit to `/` with a cached landing payload:
 * the proxy already serves signed-in users the dispatch page instead.
 */
export function SignedInRedirect() {
  const router = useRouter();
  const { bootstrapped, isAuthenticated, user } = useAuth();

  useEffect(() => {
    if (!bootstrapped || !isAuthenticated || !user) return;
    router.replace(defaultHomeForUser(user));
  }, [bootstrapped, isAuthenticated, router, user]);

  return null;
}
