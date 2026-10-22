export interface NavEntry {
  label: string;
  href: string;
  section?: string;
}

export const nav: NavEntry[] = [
  { label: 'Getting Started', href: '/getting-started' },
  { label: 'send', href: '/commands/send', section: 'Commands' },
  { label: 'receive', href: '/commands/receive', section: 'Commands' },
  { label: 'server', href: '/commands/server', section: 'Commands' },
  { label: 'See It Work', href: '/demo' },
  { label: 'Why', href: '/why' },
  { label: 'About', href: '/about' },
  { label: 'License', href: '/license' },
  { label: 'Support', href: '/support' },
  { label: 'Changelog', href: '/changelog' },
  { label: 'Testing', href: '/testing' },
];