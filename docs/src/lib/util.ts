export const topbar = 'topbar';
export const layout = 'layout';
export const sidebar = 'sidebar';
export const main = 'main-content';
export const wordmark = 'wordmark';

export function currentHref(pathname: string): string {
  const clean = pathname.replace(/\/$/, '') || '/';
  return clean;
}