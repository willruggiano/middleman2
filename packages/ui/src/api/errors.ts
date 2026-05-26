// apiErrorMessage extracts a human-readable message from an
// openapi-fetch error result. Returns the explicit `detail`, falling
// back to `title`, then to the supplied fallback. Use the fallback to
// describe what the failed action was trying to do (e.g. "sync
// failed").
export function apiErrorMessage(
  error: { detail?: string; title?: string } | undefined,
  fallback: string,
): string {
  if (!error) return fallback;
  return error.detail ?? error.title ?? fallback;
}
