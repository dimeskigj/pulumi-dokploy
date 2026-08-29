import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import { ion } from "starlight-ion-theme";
import { base, output, site } from "./src/site-config.mjs";

export default defineConfig({
  site,
  base,
  output,
  integrations: [
    starlight({
      title: "Pulumi Dokploy",
      description: "Deploy and manage Dokploy infrastructure with Pulumi.",
      plugins: [ion()],
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/dimeskigj/pulumi-dokploy",
        },
      ],
      sidebar: [
        { label: "Overview", link: "/" },
        {
          label: "Get Started",
          items: [
            { label: "Installation", link: "/getting-started/installation/" },
            { label: "First deployment", link: "/getting-started/first-deployment/" },
          ],
        },
        {
          label: "Core Concepts",
          items: [
            { label: "Projects and environments", link: "/concepts/projects-and-environments/" },
            { label: "Sources", link: "/concepts/sources/" },
            { label: "Lifecycle and state", link: "/concepts/lifecycle-and-state/" },
            { label: "Secrets", link: "/concepts/secrets/" },
          ],
        },
        {
          label: "Guides",
          items: [
            { label: "Applications", link: "/guides/applications/" },
            { label: "Compose", link: "/guides/compose/" },
            { label: "Databases", link: "/guides/databases/" },
            { label: "Domains", link: "/guides/domains/" },
            { label: "Imports", link: "/guides/imports/" },
            { label: "Troubleshooting", link: "/guides/troubleshooting/" },
          ],
        },
        {
          label: "Resources",
          items: [
            { label: "Project", link: "/reference/project/" },
            { label: "Environment", link: "/reference/environment/" },
            { label: "Application", link: "/reference/application/" },
            { label: "Compose", link: "/reference/compose/" },
            { label: "Postgres", link: "/reference/postgres/" },
            { label: "Redis", link: "/reference/redis/" },
            { label: "Domain", link: "/reference/domain/" },
            { label: "Configuration", link: "/reference/configuration/" },
            { label: "Complex Types", link: "/reference/types/" },
          ],
        },
        {
          label: "Examples",
          items: [
            { label: "Examples", link: "/examples/" },
            { label: "Complete example", link: "/examples/complete/" },
          ],
        },
        { label: "Contributing", link: "/contributing/" },
      ],
    }),
  ],
});
