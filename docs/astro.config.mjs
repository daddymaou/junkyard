import { defineConfig } from 'astro/config';
import mdx from '@astrojs/mdx';

export default defineConfig({
  integrations: [mdx()],
  markdown: {
    shikiConfig: {
      theme: './src/lib/shiki-junkyard.json',
      wrap: true,
    },
  },
});