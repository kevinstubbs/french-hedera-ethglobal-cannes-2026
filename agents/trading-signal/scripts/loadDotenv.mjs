import { config } from "dotenv";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const scriptsDir = dirname(fileURLToPath(import.meta.url));
const envPath = join(scriptsDir, "..", ".env");
// Keys present in .env override inherited shell (unlike Node’s --env-file).
config({ path: envPath, override: true, quiet: true });
