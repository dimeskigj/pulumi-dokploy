import { readFile } from "node:fs/promises";

const EXPECTED_RESOURCES = new Set([
  "Application", "Compose", "Domain", "Environment", "Postgres", "MySQL", "MariaDB", "MongoDB", "Project", "Redis",
  "Destination", "Backup", "VolumeBackup", "SSHKey", "Registry", "Tag", "ProjectTag", "Mount", "Schedule", "Port", "Notification", "Server",
