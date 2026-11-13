/** How close a token is to its expiry. */
export type ExpiryState = 'valid' | 'expiring' | 'expired' | 'none';

/** A token expiry described for display. */
export interface Expiry {
  state: ExpiryState;
  /** Short sentence such as "Expires in 4 min 32 s" or "Expired 3 min ago". */
  label: string;
  /** Local date and time of the expiry; empty without one. */
  at: string;
}

/** Below this remaining time a token is shown as about to expire. */
const EXPIRING_SOON_MILLISECONDS = 60_000;

/**
 * Describes an expiry given in milliseconds since the epoch. Zero or a missing
 * value means the server gave no expiry.
 */
export function describeExpiry(
  expiresAtMilliseconds: number | null | undefined,
  now: number = Date.now(),
): Expiry {
  if (!expiresAtMilliseconds) {
    return { state: 'none', label: 'No expiry given', at: '' };
  }

  const remaining = expiresAtMilliseconds - now;
  const at = new Date(expiresAtMilliseconds).toLocaleString();
  if (remaining <= 0) {
    return { state: 'expired', label: `Expired ${formatDuration(-remaining)} ago`, at };
  }
  return {
    state: remaining < EXPIRING_SOON_MILLISECONDS ? 'expiring' : 'valid',
    label: `Expires in ${formatDuration(remaining)}`,
    at,
  };
}

/** Formats a duration with its two largest units, such as "2 h 5 min". */
export function formatDuration(milliseconds: number): string {
  const total = Math.max(0, Math.floor(milliseconds / 1000));
  const units: [number, string][] = [
    [Math.floor(total / 86_400), 'd'],
    [Math.floor((total % 86_400) / 3600), 'h'],
    [Math.floor((total % 3600) / 60), 'min'],
    [total % 60, 's'],
  ];
  const first = units.findIndex(([value]) => value > 0);
  if (first === -1) {
    return '0 s';
  }
  return units
    .slice(first, first + 2)
    .filter(([value]) => value > 0)
    .map(([value, unit]) => `${value} ${unit}`)
    .join(' ');
}
