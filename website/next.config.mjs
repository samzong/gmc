import { createMDX } from 'fumadocs-mdx/next';

const withMDX = createMDX();

const config = {
  output: 'export',
  reactStrictMode: true,
};

export default withMDX(config);
