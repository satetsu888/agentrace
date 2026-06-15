import { describe, it, expect, beforeEach, afterEach } from "vitest";
import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";
import {
  getConfigPath,
  loadLocalConfig,
  saveLocalConfig,
  deleteLocalConfig,
  getLocalConfigPath,
  loadConfigWithFallback,
  findLocalConfigPath,
  findAndLoadLocalConfig,
  getSendMode,
  persistSendMode,
  type AgentraceConfig,
} from "./manager.js";

describe("config/manager", () => {
  const testConfig: AgentraceConfig = {
    server_url: "http://localhost:8080",
    api_key: "agtr_test_key",
  };

  describe("getSendMode", () => {
    it("returns 'sync' when config is null", () => {
      expect(getSendMode(null)).toBe("sync");
    });

    it("returns 'sync' when send_mode is not set", () => {
      expect(getSendMode(testConfig)).toBe("sync");
    });

    it("returns 'async' when send_mode is 'async'", () => {
      expect(getSendMode({ ...testConfig, send_mode: "async" })).toBe("async");
    });

    it("returns 'sync' when send_mode is 'sync'", () => {
      expect(getSendMode({ ...testConfig, send_mode: "sync" })).toBe("sync");
    });

    it("falls back to 'sync' for an unrecognized send_mode value", () => {
      expect(
        getSendMode({ ...testConfig, send_mode: "bogus" as unknown as "sync" })
      ).toBe("sync");
    });
  });

  describe("persistSendMode (local config)", () => {
    let tempProjectDir: string;

    beforeEach(() => {
      tempProjectDir = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-sendmode-"));
    });

    afterEach(() => {
      if (fs.existsSync(tempProjectDir)) {
        fs.rmSync(tempProjectDir, { recursive: true });
      }
    });

    it("sets send_mode in an existing local config", () => {
      saveLocalConfig(tempProjectDir, testConfig);

      const result = persistSendMode("async", { cwd: tempProjectDir });

      expect(result.ok).toBe(true);
      expect(loadLocalConfig(tempProjectDir)?.send_mode).toBe("async");
    });

    it("preserves other config fields when updating send_mode", () => {
      saveLocalConfig(tempProjectDir, testConfig);

      persistSendMode("async", { cwd: tempProjectDir });

      const updated = loadLocalConfig(tempProjectDir);
      expect(updated?.server_url).toBe(testConfig.server_url);
      expect(updated?.api_key).toBe(testConfig.api_key);
    });

    it("updates a local config found in a parent directory", () => {
      saveLocalConfig(tempProjectDir, testConfig);
      const subDir = path.join(tempProjectDir, "sub");
      fs.mkdirSync(subDir);

      const result = persistSendMode("async", { cwd: subDir });

      expect(result.ok).toBe(true);
      expect(loadLocalConfig(tempProjectDir)?.send_mode).toBe("async");
    });
  });

  describe("global config", () => {
    it("getConfigPath returns expected path", () => {
      expect(getConfigPath()).toBe(
        path.join(os.homedir(), ".agentrace", "config.json")
      );
    });
  });

  describe("local config", () => {
    let tempProjectDir: string;

    beforeEach(() => {
      tempProjectDir = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-project-"));
    });

    afterEach(() => {
      if (fs.existsSync(tempProjectDir)) {
        fs.rmSync(tempProjectDir, { recursive: true });
      }
    });

    it("getLocalConfigPath returns correct path", () => {
      const configPath = getLocalConfigPath(tempProjectDir);
      expect(configPath).toBe(path.join(tempProjectDir, ".agentrace", "config.json"));
    });

    it("saveLocalConfig creates config file", () => {
      saveLocalConfig(tempProjectDir, testConfig);

      const configPath = getLocalConfigPath(tempProjectDir);
      expect(fs.existsSync(configPath)).toBe(true);

      const content = JSON.parse(fs.readFileSync(configPath, "utf-8"));
      expect(content).toEqual(testConfig);
    });

    it("loadLocalConfig returns config when exists", () => {
      saveLocalConfig(tempProjectDir, testConfig);

      const loaded = loadLocalConfig(tempProjectDir);
      expect(loaded).toEqual(testConfig);
    });

    it("loadLocalConfig returns null when config does not exist", () => {
      const loaded = loadLocalConfig(tempProjectDir);
      expect(loaded).toBeNull();
    });

    it("deleteLocalConfig removes config directory", () => {
      saveLocalConfig(tempProjectDir, testConfig);
      const configDir = path.join(tempProjectDir, ".agentrace");
      expect(fs.existsSync(configDir)).toBe(true);

      const result = deleteLocalConfig(tempProjectDir);
      expect(result).toBe(true);
      expect(fs.existsSync(configDir)).toBe(false);
    });

    it("deleteLocalConfig returns false when no config exists", () => {
      const result = deleteLocalConfig(tempProjectDir);
      expect(result).toBe(false);
    });
  });

  describe("loadConfigWithFallback", () => {
    let tempProjectDir: string;

    const localConfig: AgentraceConfig = {
      server_url: "http://local:8080",
      api_key: "test_local_key",
    };

    beforeEach(() => {
      tempProjectDir = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-project-"));
    });

    afterEach(() => {
      if (fs.existsSync(tempProjectDir)) {
        fs.rmSync(tempProjectDir, { recursive: true });
      }
    });

    it("returns local config when it exists", () => {
      saveLocalConfig(tempProjectDir, localConfig);

      const loaded = loadConfigWithFallback(tempProjectDir);
      expect(loaded).toEqual(localConfig);
    });

    it("prefers local config over global config", () => {
      // Save local config
      saveLocalConfig(tempProjectDir, localConfig);

      // loadConfigWithFallback should return local config
      const loaded = loadConfigWithFallback(tempProjectDir);
      expect(loaded).toEqual(localConfig);
    });
  });

  describe("findLocalConfigPath", () => {
    let tempRootDir: string;
    let tempSubDir: string;
    let tempDeepDir: string;

    const localConfig: AgentraceConfig = {
      server_url: "http://local:8080",
      api_key: "test_local_key",
    };

    beforeEach(() => {
      // Create nested directory structure: root/sub/deep
      tempRootDir = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-root-"));
      tempSubDir = path.join(tempRootDir, "sub");
      tempDeepDir = path.join(tempSubDir, "deep");
      fs.mkdirSync(tempDeepDir, { recursive: true });
    });

    afterEach(() => {
      if (fs.existsSync(tempRootDir)) {
        fs.rmSync(tempRootDir, { recursive: true });
      }
    });

    it("finds config in current directory", () => {
      saveLocalConfig(tempRootDir, localConfig);

      const foundPath = findLocalConfigPath(tempRootDir);
      expect(foundPath).toBe(getLocalConfigPath(tempRootDir));
    });

    it("finds config in parent directory when searching from subdirectory", () => {
      saveLocalConfig(tempRootDir, localConfig);

      const foundPath = findLocalConfigPath(tempSubDir);
      expect(foundPath).toBe(getLocalConfigPath(tempRootDir));
    });

    it("finds config in ancestor directory when searching from deep subdirectory", () => {
      saveLocalConfig(tempRootDir, localConfig);

      const foundPath = findLocalConfigPath(tempDeepDir);
      expect(foundPath).toBe(getLocalConfigPath(tempRootDir));
    });

    it("prefers nearest config when multiple exist in ancestor chain", () => {
      const rootConfig: AgentraceConfig = {
        server_url: "http://root:8080",
        api_key: "root_key",
      };
      const subConfig: AgentraceConfig = {
        server_url: "http://sub:8080",
        api_key: "sub_key",
      };

      saveLocalConfig(tempRootDir, rootConfig);
      saveLocalConfig(tempSubDir, subConfig);

      // From deep, should find sub's config (nearest ancestor)
      const foundPath = findLocalConfigPath(tempDeepDir);
      expect(foundPath).toBe(getLocalConfigPath(tempSubDir));
    });

    it("returns null when no config exists in ancestor chain", () => {
      const foundPath = findLocalConfigPath(tempDeepDir);
      expect(foundPath).toBeNull();
    });
  });

  describe("findAndLoadLocalConfig", () => {
    let tempRootDir: string;
    let tempSubDir: string;

    const localConfig: AgentraceConfig = {
      server_url: "http://local:8080",
      api_key: "test_local_key",
    };

    beforeEach(() => {
      tempRootDir = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-root-"));
      tempSubDir = path.join(tempRootDir, "sub");
      fs.mkdirSync(tempSubDir, { recursive: true });
    });

    afterEach(() => {
      if (fs.existsSync(tempRootDir)) {
        fs.rmSync(tempRootDir, { recursive: true });
      }
    });

    it("returns config and path when found in parent directory", () => {
      saveLocalConfig(tempRootDir, localConfig);

      const result = findAndLoadLocalConfig(tempSubDir);
      expect(result).not.toBeNull();
      expect(result!.config).toEqual(localConfig);
      expect(result!.path).toBe(getLocalConfigPath(tempRootDir));
    });

    it("returns null when no config exists", () => {
      const result = findAndLoadLocalConfig(tempSubDir);
      expect(result).toBeNull();
    });
  });

  describe("loadConfigWithFallback with parent directory search", () => {
    let tempRootDir: string;
    let tempSubDir: string;

    const localConfig: AgentraceConfig = {
      server_url: "http://local:8080",
      api_key: "test_local_key",
    };

    beforeEach(() => {
      tempRootDir = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-root-"));
      tempSubDir = path.join(tempRootDir, "sub");
      fs.mkdirSync(tempSubDir, { recursive: true });
    });

    afterEach(() => {
      if (fs.existsSync(tempRootDir)) {
        fs.rmSync(tempRootDir, { recursive: true });
      }
    });

    it("finds config in parent directory when called from subdirectory", () => {
      saveLocalConfig(tempRootDir, localConfig);

      const loaded = loadConfigWithFallback(tempSubDir);
      expect(loaded).toEqual(localConfig);
    });
  });
});
