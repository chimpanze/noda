export interface MiddlewareConfigLocation {
  configPath: string;
  configKey: string;
}

const SPECIAL_MAPPINGS: Record<string, MiddlewareConfigLocation> = {
  "auth.jwt": { configPath: "security", configKey: "jwt" },
  "casbin.enforce": { configPath: "security", configKey: "casbin" },
  "livekit.webhook": { configPath: "security", configKey: "livekit" },
};

export function resolveMiddlewareConfigPath(
  name: string,
): MiddlewareConfigLocation {
  if (SPECIAL_MAPPINGS[name]) {
    return SPECIAL_MAPPINGS[name];
  }
  if (name.startsWith("security.")) {
    return {
      configPath: "security",
      configKey: name.replace("security.", ""),
    };
  }
  return { configPath: "middleware", configKey: name };
}

/**
 * Read raw middleware config from rootConfig using the resolved path.
 */
export function extractMiddlewareConfig(
  rootConfig: Record<string, unknown>,
  name: string,
): Record<string, unknown> {
  const { configPath, configKey } = resolveMiddlewareConfigPath(name);
  const section = rootConfig[configPath] as
    | Record<string, unknown>
    | undefined;
  return { ...((section?.[configKey] as Record<string, unknown>) ?? {}) };
}

/**
 * Write middleware config into a cloned rootConfig at the resolved path.
 * Returns the updated rootConfig.
 */
export function writeMiddlewareConfig(
  rootConfig: Record<string, unknown>,
  name: string,
  config: Record<string, unknown>,
): Record<string, unknown> {
  const updated = structuredClone(rootConfig);
  const { configPath, configKey } = resolveMiddlewareConfigPath(name);
  const hasValues = Object.keys(config).length > 0;

  const section = (updated[configPath] ?? {}) as Record<string, unknown>;
  if (hasValues) {
    section[configKey] = config;
  } else {
    delete section[configKey];
  }
  updated[configPath] =
    Object.keys(section).length > 0 ? section : undefined;

  return updated;
}
