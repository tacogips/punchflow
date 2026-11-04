export function parseLinkHeader(header: string | null): Record<string, string> {
  if (!header) return {};
  const links: Record<string, string> = {};
  for (const part of header.split(/,(?=\s*<)/)) {
    const match = part.trim().match(/^<([^>]+)>\s*;\s*rel="?([^";]+)"?/);
    if (match) links[match[2]] = match[1];
  }
  return links;
}
