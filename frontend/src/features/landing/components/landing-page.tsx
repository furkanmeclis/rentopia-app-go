import { ClosingSection } from "./closing-section";
import { ConnectSection } from "./connect-section";
import { ControlSection } from "./control-section";
import { FaqSection } from "./faq-section";
import { FeatureRows } from "./feature-rows";
import { FlowSection } from "./flow-section";
import { LandingFooter } from "./landing-footer";
import { LandingHeader } from "./landing-header";
import { LandingHero } from "./landing-hero";
import { SignedInRedirect } from "./signed-in-redirect";
import { LandingMotion } from "./motion-primitives";
import { SiteSection } from "./site-section";
import { StatementSection } from "./statement-section";

/** Public marketing page at `/` for signed-out visitors. */
export function LandingPage() {
  return (
    <LandingMotion>
      <div className="bg-background text-foreground min-h-svh">
        <SignedInRedirect />
        <LandingHeader />
        <main id="main" tabIndex={-1} className="outline-none">
          <LandingHero />
          <StatementSection />
          <FlowSection />
          <FeatureRows />
          <SiteSection />
          <ControlSection />
          <ConnectSection />
          <FaqSection />
          <ClosingSection />
        </main>
        <LandingFooter />
      </div>
    </LandingMotion>
  );
}
