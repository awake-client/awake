/** Shows a release version as "v0.0.4", and development builds as "dev". */
export function formatVersion(version: string | null | undefined): string {
  if (!version || version === 'dev') {
    return 'dev';
  }
  return `v${version.replace(/^v/i, '')}`;
}
