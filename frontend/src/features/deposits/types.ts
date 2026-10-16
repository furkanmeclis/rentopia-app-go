import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type SecurityDeposit = Schemas["SecurityDeposit"];
export type DepositDeduction = Schemas["DepositDeduction"];
export type SecurityDepositInput = Schemas["SecurityDepositInput"];
export type SecurityDepositResolveInput =
  Schemas["SecurityDepositResolveInput"];
export type DepositDeductionInput = Schemas["DepositDeductionInput"];
export type DepositTotals = Schemas["DepositTotals"];
export type RentalDeposits = Schemas["RentalDeposits"];
export type DepositCashAccount = Schemas["DepositCashAccount"];

export type DepositMethod = SecurityDeposit["method"];
export type DepositStatus = SecurityDeposit["status"];
export type DeductionTarget = DepositDeduction["applies_to"];
