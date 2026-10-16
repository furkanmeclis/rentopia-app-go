import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import Script from "next/script";

import { brand } from "@/config/brand";
import {
  ISSUE_WIDGET_ENVIRONMENT,
  ISSUE_WIDGET_RELEASE,
  ISSUE_WIDGET_SRC,
} from "@/lib/issue-widget";
import { AppProviders } from "@/providers/app-providers";

import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin", "latin-ext"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin", "latin-ext"],
});

export const metadata: Metadata = {
  title: {
    default: brand.productName,
    template: `%s · ${brand.name}`,
  },
  description: brand.tagline,
  applicationName: brand.productName,
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="tr" suppressHydrationWarning className="h-full">
      <body
        className={`${geistSans.variable} ${geistMono.variable} min-h-full font-sans antialiased`}
      >
        {/* Runtime browser config (Sentry DSN); must run before hydration. */}
        <Script src="/api/client-env" strategy="beforeInteractive" />
        {/* Feedback widget (Linear issue requests), loaded before hydration
            so it sees page-load errors. It comes after client-env because
            Next loads beforeInteractive scripts in sequence and skips the
            rest when one fails: a blocked widget must not drop the Sentry
            config. Sentry's instrumentation-client wraps console / fetch /
            XHR earlier; the widget wraps on top and both pass calls through. */}
        <Script
          src={ISSUE_WIDGET_SRC}
          strategy="beforeInteractive"
          data-release={ISSUE_WIDGET_RELEASE}
          data-environment={ISSUE_WIDGET_ENVIRONMENT}
          data-label="Geri bildirim"
        />
        <AppProviders>{children}</AppProviders>
      </body>
    </html>
  );
}
