/** Injects runtime API URL into the browser (from container env API_URL). */
export function ApiRuntimeConfig({ apiUrl }: { apiUrl: string }) {
  if (!apiUrl) return null;
  return (
    <script
      dangerouslySetInnerHTML={{
        __html: `window.__SWFM_API_BASE__=${JSON.stringify(apiUrl.replace(/\/+$/, ''))};`,
      }}
    />
  );
}
