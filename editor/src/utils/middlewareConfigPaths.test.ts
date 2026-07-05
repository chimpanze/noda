import { describe, expect, it } from "vitest";
import {
  extractMiddlewareConfig,
  resolveMiddlewareConfigPath,
  writeMiddlewareConfig,
} from "./middlewareConfigPaths";

describe("resolveMiddlewareConfigPath", () => {
  // Must stay in sync with middlewareConfigPaths in internal/server/middleware.go
  it.each([
    ["auth.jwt", "security", "jwt"],
    ["auth.oidc", "security", "oidc"],
    ["auth.session", "security", "session"],
    ["casbin.enforce", "security", "casbin"],
    ["livekit.webhook", "security", "livekit"],
    ["security.cors", "security", "cors"],
    ["security.headers", "security", "headers"],
    ["security.csrf", "security", "csrf"],
  ])("%s → %s.%s (server-side path)", (name, configPath, configKey) => {
    expect(resolveMiddlewareConfigPath(name)).toEqual({ configPath, configKey });
  });

  it("defaults to the middleware section", () => {
    expect(resolveMiddlewareConfigPath("limiter")).toEqual({
      configPath: "middleware",
      configKey: "limiter",
    });
  });
});

describe("extract/write round trip", () => {
  it("reads and writes auth.session under security.session", () => {
    const root = writeMiddlewareConfig({}, "auth.session", { cookie: "noda_session" });
    expect(
      (root.security as Record<string, unknown>).session,
    ).toEqual({ cookie: "noda_session" });
    expect(extractMiddlewareConfig(root, "auth.session")).toEqual({
      cookie: "noda_session",
    });
  });
});
