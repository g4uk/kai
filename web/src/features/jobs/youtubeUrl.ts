// Pure client-side YouTube URL validator (specs/ui/plan.md step 6).
// Approximates internal/handler/jobs.go's isValidYoutubeURL closely enough
// to reject obvious garbage; the server remains authoritative (criterion 9)
// since this is a UX gate only, not the source of truth.

/** Returns true if `raw` looks like a recognizable YouTube video URL. */
export function isValidYoutubeUrl(raw: string): boolean {
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return false;
  }

  if (url.protocol !== "https:" && url.protocol !== "http:") {
    return false;
  }

  const host = url.hostname.replace(/^www\./i, "").toLowerCase();

  if (host === "youtu.be") {
    return /^\/[\w-]+\/?$/.test(url.pathname);
  }

  if (host === "youtube.com") {
    if (url.pathname === "/watch") {
      return (url.searchParams.get("v") ?? "") !== "";
    }
    return /^\/shorts\/[\w-]+\/?$/.test(url.pathname);
  }

  return false;
}
