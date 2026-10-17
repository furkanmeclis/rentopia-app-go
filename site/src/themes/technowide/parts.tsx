import {
  Award,
  BadgeCheck,
  Calendar,
  Car,
  Check,
  CheckCircle,
  CircleCheck,
  Clock,
  CreditCard,
  Fuel,
  Globe,
  Headphones,
  Heart,
  KeyRound,
  MapPin,
  Phone,
  Plane,
  Route,
  ShieldCheck,
  Star,
  ThumbsUp,
  Users,
  Wallet,
  Wrench,
  Zap,
  type LucideIcon,
} from "lucide-react";

/** The API's advantage icon allowlist (kebab-case Lucide names). */
export const ADVANTAGE_ICONS: Record<string, LucideIcon> = {
  "shield-check": ShieldCheck,
  clock: Clock,
  car: Car,
  "credit-card": CreditCard,
  headphones: Headphones,
  "map-pin": MapPin,
  check: Check,
  "check-circle": CheckCircle,
  star: Star,
  heart: Heart,
  award: Award,
  wallet: Wallet,
  zap: Zap,
  users: Users,
  phone: Phone,
  "thumbs-up": ThumbsUp,
  "badge-check": BadgeCheck,
  "circle-check": CircleCheck,
  "key-round": KeyRound,
  fuel: Fuel,
  plane: Plane,
  route: Route,
  wrench: Wrench,
  calendar: Calendar,
  globe: Globe,
};

/** Kicker line with the short rule ("— YOL ARKADAŞINI SEÇ"). */
export function Eyebrow({ children }: { children: React.ReactNode }) {
  return (
    <p className="tw-eyebrow">
      <span aria-hidden />
      {children}
    </p>
  );
}

/** Section heading: kicker + title on the left, lead and action on the right. */
export function SectionHeading({
  id,
  eyebrow,
  title,
  lead,
  action,
}: {
  id: string;
  eyebrow: string;
  title: string;
  lead?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="tw-section-heading">
      <div>
        <Eyebrow>{eyebrow}</Eyebrow>
        <h2 id={id}>{title}</h2>
      </div>
      {lead || action ? (
        <div className="tw-section-heading-end">
          {lead ? <p>{lead}</p> : null}
          {action}
        </div>
      ) : null}
    </div>
  );
}
