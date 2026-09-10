import assert from "node:assert/strict";
import { once } from "node:events";
import { mkdir, mkdtemp, readFile, readdir, rm, stat, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import test from "node:test";

const execFileAsync = promisify(execFile);
const repoRoot = resolve(import.meta.dirname, "../..");
const installer = join(repoRoot, "build-opencode-plugin.sh");

type Scenario = "success" | "reuse" | "reject-login" | "malformed-key";

async function startGateway(scenario: Scenario, validKeys: string[] = []) {
  const calls: Array<{ path: string; body: string; authorization?: string; cookie?: string }> = [];
  const valid = new Set(validKeys);
  const server = createServer(async (req, res) => {
    const chunks: Buffer[] = [];
    for await (const chunk of req) chunks.push(Buffer.from(chunk));
    const body = Buffer.concat(chunks).toString("utf8");
    calls.push({
      path: req.url ?? "",
      body,
      authorization: req.headers.authorization,
      cookie: req.headers.cookie,
    });

    if (req.url === "/api/auth/status") {
      res.setHeader("Content-Type", "application/json");
      return void res.end("{}");
    }
    if (req.url === "/v1/models") {
      const key = req.headers.authorization?.replace(/^Bearer /, "");
      if (!key || !valid.has(key)) {
        res.statusCode = 401;
        return void res.end('{"error":"unauthorized"}');
      }
      res.setHeader("Content-Type", "application/json");
      return void res.end('{"data":[]}');
    }
    if (req.url === "/api/auth/login") {
      assert.deepEqual(JSON.parse(body), { password: "dashboard-password" });
      if (scenario === "reject-login") {
        res.statusCode = 401;
        return void res.end('{"error":"invalid password"}');
      }
      res.setHeader("Set-Cookie", "kr_session=test-session; Path=/; HttpOnly");
      res.setHeader("Content-Type", "application/json");
      return void res.end('{"ok":true}');
    }
    if (req.url === "/api/keys") {
      assert.match(req.headers.cookie ?? "", /kr_session=test-session/);
      assert.deepEqual(JSON.parse(body), { name: "OpenCode" });
      res.setHeader("Content-Type", "application/json");
      if (scenario === "malformed-key") return void res.end('{"key":"not-a-keirouter-key"}');
      valid.add("kr_created");
      return void res.end('{"key":"kr_created"}');
    }
    res.statusCode = 404;
    res.end();
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const address = server.address();
  assert(address && typeof address !== "string");
  return { server, calls, url: `http://127.0.0.1:${address.port}` };
}

async function runInstaller(root: string, url: string) {
  const config = join(root, "config", "opencode");
  const data = join(root, "data", "opencode");
  const temporary = join(root, "tmp");
  await mkdir(temporary, { recursive: true });
  try {
    const result = await execFileAsync("sh", [installer], {
      cwd: repoRoot,
      env: {
        ...process.env,
        HOME: join(root, "home"),
        OPENCODE_CONFIG_DIR: config,
        OPENCODE_DATA_DIR: data,
        TMPDIR: temporary,
        KEIROUTER_URL: url,
        KEIROUTER_DASHBOARD_PASSWORD: "dashboard-password",
      },
    });
    return { ...result, config, data, temporary };
  } catch (error) {
    const failure = error as { stdout?: string; stderr?: string; code?: number };
    return { stdout: failure.stdout ?? "", stderr: failure.stderr ?? "", code: failure.code, config, data, temporary };
  }
}

test("installer provisions, validates, and preserves OpenCode credentials", async (t) => {
  await t.test("creates and saves a newly issued key", async () => {
    const root = await mkdtemp(join(tmpdir(), "keirouter-opencode-install-"));
    const gateway = await startGateway("success");
    try {
      const result = await runInstaller(root, gateway.url);
      assert.equal(result.code, undefined, result.stderr);
      const auth = JSON.parse(await readFile(join(result.data, "auth.json"), "utf8"));
      assert.deepEqual(auth.keirouter, { type: "api", key: "kr_created" });
      assert.equal((await stat(join(result.data, "auth.json"))).mode & 0o777, 0o600);
      assert.equal(await readFile(join(result.config, "plugins", "keirouter-plugin.js"), "utf8").then(Boolean), true);
      assert.deepEqual((await readdir(result.temporary)).filter((name) => name.startsWith("keirouter-opencode-")), []);
      assert.deepEqual(gateway.calls.map((call) => call.path), [
        "/api/auth/status",
        "/api/auth/login",
        "/api/keys",
        "/v1/models",
      ]);
      assert.doesNotMatch(`${result.stdout}${result.stderr}`, /kr_created|dashboard-password/);
    } finally {
      gateway.server.close();
      await rm(root, { recursive: true, force: true });
    }
  });

  await t.test("reuses a valid key and retains unrelated auth entries", async () => {
    const root = await mkdtemp(join(tmpdir(), "keirouter-opencode-install-"));
    const gateway = await startGateway("reuse", ["kr_saved"]);
    const authFile = join(root, "data", "opencode", "auth.json");
    await mkdir(join(root, "data", "opencode"), { recursive: true });
    await writeFile(authFile, JSON.stringify({ other: { type: "oauth" }, keirouter: { type: "api", key: "kr_saved" } }), { mode: 0o600 });
    try {
      const result = await runInstaller(root, gateway.url);
      assert.equal(result.code, undefined, result.stderr);
      assert.deepEqual(JSON.parse(await readFile(authFile, "utf8")), {
        other: { type: "oauth" },
        keirouter: { type: "api", key: "kr_saved" },
      });
      assert.deepEqual(gateway.calls.map((call) => call.path), ["/api/auth/status", "/v1/models"]);
    } finally {
      gateway.server.close();
      await rm(root, { recursive: true, force: true });
    }
  });

  await t.test("replaces an invalid key while retaining unrelated auth entries", async () => {
    const root = await mkdtemp(join(tmpdir(), "keirouter-opencode-install-"));
    const gateway = await startGateway("success");
    const authFile = join(root, "data", "opencode", "auth.json");
    await mkdir(join(root, "data", "opencode"), { recursive: true });
    await writeFile(authFile, JSON.stringify({ other: { type: "oauth" }, keirouter: { type: "api", key: "kr_stale" } }), { mode: 0o600 });
    try {
      const result = await runInstaller(root, gateway.url);
      assert.equal(result.code, undefined, result.stderr);
      assert.deepEqual(JSON.parse(await readFile(authFile, "utf8")), {
        other: { type: "oauth" },
        keirouter: { type: "api", key: "kr_created" },
      });
      assert.deepEqual(gateway.calls.map((call) => call.path), [
        "/api/auth/status",
        "/v1/models",
        "/api/auth/login",
        "/api/keys",
        "/v1/models",
      ]);
    } finally {
      gateway.server.close();
      await rm(root, { recursive: true, force: true });
    }
  });

  await t.test("does not overwrite credentials after rejected login or malformed key response", async () => {
    for (const scenario of ["reject-login", "malformed-key"] as const) {
      const root = await mkdtemp(join(tmpdir(), "keirouter-opencode-install-"));
      const gateway = await startGateway(scenario);
      const authFile = join(root, "data", "opencode", "auth.json");
      const original = { other: { type: "oauth" }, keirouter: { type: "api", key: "kr_stale" } };
      await mkdir(join(root, "data", "opencode"), { recursive: true });
      await writeFile(authFile, JSON.stringify(original), { mode: 0o600 });
      try {
        const result = await runInstaller(root, gateway.url);
        assert.notEqual(result.code, undefined, `${scenario} unexpectedly succeeded`);
        assert.deepEqual(JSON.parse(await readFile(authFile, "utf8")), original);
        assert.doesNotMatch(`${result.stdout}${result.stderr}`, /dashboard-password/);
      } finally {
        gateway.server.close();
        await rm(root, { recursive: true, force: true });
      }
    }
  });
});

test("installer rejects a non-loopback KeiRouter URL before requesting credentials", async () => {
  const root = await mkdtemp(join(tmpdir(), "keirouter-opencode-install-"));
  try {
    const result = await runInstaller(root, "http://example.com:20180");
    assert.notEqual(result.code, undefined);
    assert.match(result.stderr, /loopback HTTP URL/);
    assert.doesNotMatch(`${result.stdout}${result.stderr}`, /dashboard-password/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("installer fails safely when the local KeiRouter API is unavailable", async () => {
  const root = await mkdtemp(join(tmpdir(), "keirouter-opencode-install-"));
  try {
    const result = await runInstaller(root, "http://127.0.0.1:1");
    assert.notEqual(result.code, undefined);
    assert.match(result.stderr, /not reachable/);
    assert.doesNotMatch(`${result.stdout}${result.stderr}`, /dashboard-password/);
    assert.equal(await readFile(join(result.data, "auth.json"), "utf8").then(() => true, () => false), false);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
