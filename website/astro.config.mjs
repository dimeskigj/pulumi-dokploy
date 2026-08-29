import { defineConfig } from "astro/config";

// Starlight is published as TypeScript. Astro loads it through its config
// bundler, while the Node configuration test only needs to inspect the shape.
const starlight = process.env.NODE_TEST_CONTEXT || process.argv.includes("--test")
  ? () => ({ name: "@astrojs/starlight" })
  : (await import("@astrojs/starlight")).default;

export default defineConfig({
  site: "https://gjorgjidimeski.github.io",
  base: "/pulumi-dokploy",
  output: "static",
  integrations: [
    starlight({
      title: "Pulumi Dokploy",
      description: "Deploy and manage Dokploy infrastructure with Pulumi.",
      customCss: ["./src/styles/global.css"],
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/gjorgjidimeski/pulumi-dokploy",
        },
      ],
      sidebar: [{ label: "Overview", link: "/" }],
    }),
  ],
});
