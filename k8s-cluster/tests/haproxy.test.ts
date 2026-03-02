import { describe, it, expect } from "bun:test";
import { readFileSync } from "fs";
import { join } from "path";

const HAPROXY_CONFIG_PATH = join(import.meta.dir, "..", "haproxy", "haproxy.cfg");

function extractSection(config: string, sectionName: string): string | null {
  // Split config into sections by lines starting at column 0 with known keywords
  const lines = config.split("\n");
  let capturing = false;
  let section: string[] = [];

  for (const line of lines) {
    // Check if this line starts a new top-level section
    if (/^(global|defaults|frontend|backend|listen)\s/.test(line) || line === "global" || line === "defaults") {
      if (capturing) {
        // We were capturing our target section, now it ends
        break;
      }
      // Check if this is the section we want
      if (line.startsWith(sectionName)) {
        capturing = true;
        section.push(line);
        continue;
      }
    }
    if (capturing) {
      section.push(line);
    }
  }

  return section.length > 0 ? section.join("\n") : null;
}

describe("HAProxy Configuration", () => {
  const config = readFileSync(HAPROXY_CONFIG_PATH, "utf-8");

  it("should have a global section", () => {
    expect(config).toContain("global");
    expect(config).toContain("maxconn");
  });

  it("should have a defaults section with timeouts", () => {
    expect(config).toContain("defaults");
    expect(config).toContain("timeout connect");
    expect(config).toContain("timeout client");
    expect(config).toContain("timeout server");
  });

  it("should use least-connection algorithm for api_servers", () => {
    const section = extractSection(config, "backend api_servers");
    expect(section).not.toBeNull();
    expect(section!).toContain("balance leastconn");
  });

  it("should use least-connection algorithm for ws_servers", () => {
    const section = extractSection(config, "backend ws_servers");
    expect(section).not.toBeNull();
    expect(section!).toContain("balance leastconn");
  });

  it("should have health checks on api backend", () => {
    const section = extractSection(config, "backend api_servers");
    expect(section).not.toBeNull();
    expect(section!).toContain("option httpchk");
    expect(section!).toContain("check");
  });

  it("should have health checks on ws backend", () => {
    const section = extractSection(config, "backend ws_servers");
    expect(section).not.toBeNull();
    expect(section!).toContain("option httpchk");
    expect(section!).toContain("check");
  });

  it("should have an HTTP frontend on port 80", () => {
    expect(config).toContain("frontend http_front");
    expect(config).toContain("bind *:80");
  });

  it("should have a WebSocket frontend on port 8080", () => {
    expect(config).toContain("frontend ws_front");
    expect(config).toContain("bind *:8080");
  });

  it("should have a stats listener on port 9090", () => {
    expect(config).toContain("listen stats");
    expect(config).toContain("bind *:9090");
    expect(config).toContain("stats enable");
  });

  it("should route WebSocket upgrades to ws_servers", () => {
    expect(config).toContain("acl is_websocket");
    expect(config).toContain("use_backend ws_servers");
  });

  it("should have 2 API server entries", () => {
    const section = extractSection(config, "backend api_servers");
    expect(section).not.toBeNull();
    const serverLines = section!
      .split("\n")
      .filter((l) => l.trim().startsWith("server "));
    expect(serverLines.length).toBe(2);
  });

  it("should have tunnel timeout for WebSocket connections", () => {
    expect(config).toContain("timeout tunnel");
  });
});
