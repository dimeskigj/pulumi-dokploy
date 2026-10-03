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
            { label: "Backups", link: "/guides/backups/" },
            { label: "Schedules", link: "/guides/schedules/" },
            { label: "Imports", link: "/guides/imports/" },
            { label: "Lookups", link: "/guides/lookups/" },
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
            { label: "MySQL", link: "/reference/mysql/" },
            { label: "MariaDB", link: "/reference/mariadb/" },
            { label: "MongoDB", link: "/reference/mongodb/" },
            { label: "Redis", link: "/reference/redis/" },
            { label: "Domain", link: "/reference/domain/" },
            { label: "Destination", link: "/reference/destination/" },
            { label: "Backup", link: "/reference/backup/" },
            { label: "VolumeBackup", link: "/reference/volume-backup/" },
            { label: "Schedule", link: "/reference/schedule/" },
            { label: "SSHKey", link: "/reference/sshkey/" },
            { label: "Registry", link: "/reference/registry/" },
            { label: "Tag", link: "/reference/tag/" },
            { label: "ProjectTag", link: "/reference/project-tag/" },
            { label: "Mount", link: "/reference/mount/" },
            { label: "Configuration", link: "/reference/configuration/" },
            { label: "Complex Types", link: "/reference/types/" },
          ],
        },
        {
          label: "Functions",
          items: [
            { label: "getProject", link: "/reference/get-project/" },
            { label: "getEnvironment", link: "/reference/get-environment/" },
            { label: "getApplication", link: "/reference/get-application/" },
            { label: "getCompose", link: "/reference/get-compose/" },
            { label: "getPostgres", link: "/reference/get-postgres/" },
            { label: "getMySQL", link: "/reference/get-mysql/" },
            { label: "getMariaDB", link: "/reference/get-mariadb/" },
            { label: "getMongoDB", link: "/reference/get-mongodb/" },
            { label: "getRedis", link: "/reference/get-redis/" },
            { label: "getServer", link: "/reference/get-server/" },
            { label: "getRegistry", link: "/reference/get-registry/" },
            { label: "getSSHKey", link: "/reference/get-ssh-key/" },
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
