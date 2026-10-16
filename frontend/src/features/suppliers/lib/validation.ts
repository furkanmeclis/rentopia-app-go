/**
 * Client-side mirrors of the API's tax number checks (backend pii.ValidVKN /
 * pii.ValidTCKN): early feedback in the form; the API stays authoritative.
 */

/** VKN: 10 digits with the GİB checksum. */
export function validVKN(v: string): boolean {
  if (!/^\d{10}$/.test(v)) return false;
  let sum = 0;
  for (let i = 0; i < 9; i++) {
    const tmp = (Number(v[i]) + 9 - i) % 10;
    if (tmp === 0) continue;
    let p = (tmp * 2 ** (9 - i)) % 9;
    if (p === 0) p = 9;
    sum += p;
  }
  return (10 - (sum % 10)) % 10 === Number(v[9]);
}

/** TCKN: 11 digits, no leading zero, two check digits. */
export function validTCKN(v: string): boolean {
  if (!/^[1-9]\d{10}$/.test(v)) return false;
  const d = v.split("").map(Number);
  const odd = d[0] + d[2] + d[4] + d[6] + d[8];
  const even = d[1] + d[3] + d[5] + d[7];
  const d10 = (((odd * 7 - even) % 10) + 10) % 10;
  if (d10 !== d[9]) return false;
  const total = d.slice(0, 10).reduce((a, b) => a + b, 0);
  return total % 10 === d[10];
}

/** A VKN (10 digits) or TCKN (11 digits) with a valid checksum. */
export function validTaxNumber(v: string): boolean {
  return v.length === 10 ? validVKN(v) : validTCKN(v);
}
