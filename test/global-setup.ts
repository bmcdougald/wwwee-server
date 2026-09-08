import { execSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { BASE_URL, CONTAINER_NAME, IMAGE_TAG, TEST_PORT } from "./config";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

function sh(cmd: string) {
  execSync(cmd, { cwd: repoRoot, stdio: "inherit" });
}

async function waitForHealthy(timeoutMs = 60_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${BASE_URL}/healthz`);
      if (res.ok) return;
    } catch {
      // server not accepting connections yet, keep polling
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`server did not become healthy within ${timeoutMs}ms`);
}

// Vitest globalSetup: builds the hardened container image, runs it with the
// same restrictions as production (read-only rootfs, all caps dropped, no
// new privileges), and mounts the repository's sample content as a test
// fixture. Production site content is supplied by external persistent storage.
export default async function setup() {
  try {
    execSync(`docker rm -f ${CONTAINER_NAME}`, {
      cwd: repoRoot,
      stdio: "ignore",
    });
  } catch {
    // no pre-existing container from a previous failed run
  }

  sh(`docker build -t ${IMAGE_TAG} .`);
  sh(
    `docker run -d --rm --name ${CONTAINER_NAME} -p ${TEST_PORT}:8080 ` +
      `--read-only --cap-drop=ALL --security-opt no-new-privileges ` +
      `--tmpfs /tmp:mode=1700 ` +
      `-v "${path.join(repoRoot, "content")}:/srv/content:ro" ${IMAGE_TAG}`,
  );

  await waitForHealthy();

  return async () => {
    try {
      execSync(`docker stop ${CONTAINER_NAME}`, {
        cwd: repoRoot,
        stdio: "ignore",
      });
    } catch {
      // already stopped
    }
  };
}
