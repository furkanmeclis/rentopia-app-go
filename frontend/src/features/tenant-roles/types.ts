import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type TenantRole = Schemas["TenantRole"];
export type TenantRolePage = Schemas["TenantRolePage"];
export type TenantRoleRequest = Schemas["TenantRoleRequest"];
export type TenantPermission = Schemas["TenantPermission"];

/** List query of `GET /v1/tenant/roles`. */
export type ListTenantRolesParams = {
  limit: number;
  offset: number;
  q?: string;
  is_template?: "true" | "false";
};
