export { auth as proxy } from "@/auth";

export const config = {
  matcher: [
    "/",
    "/home",
    "/profile/:path*",
    "/organizations",
    "/no-access",
    "/platform/:path*",
    "/t/:path*",
  ],
};
