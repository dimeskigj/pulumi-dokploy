import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import { base, output, site } from "./src/site-config.mjs";

export default defineConfig({
  site,
  base,
  output,
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
