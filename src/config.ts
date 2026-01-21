/**
 * Configuration Loader
 * 
 * Loads configuration from environment variables and config file.
 * Priority: env vars > config file
 */

import * as fs from "fs/promises";
import * as path from "path";

const CONFIG_DIR = `${process.env.HOME}/.config/agenator`;
const CONFIG_FILE = "config.yaml";

export interface AgenatorConfig {
  linear?: {
    apiKey?: string;
    defaultTeam?: string;
  };
  agent?: {
    command?: string;
    args?: string[];
  };
  workspace?: {
    baseDir?: string;
    sourceRepo?: string;
    branchPrefix?: string;
    cleanupOnMerge?: boolean;
  };
}

/**
 * Simple YAML parser for our config format
 * Handles basic key: value pairs with optional nesting
 */
function parseYaml(content: string): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  const lines = content.split("\n");
  let currentSection: string | null = null;

  for (const line of lines) {
    // Skip comments and empty lines
    if (line.trim().startsWith("#") || line.trim() === "") continue;

    // Count leading spaces
    const indent = line.search(/\S/);
    const trimmed = line.trim();
    
    // Parse key: value
    const colonIndex = trimmed.indexOf(":");
    if (colonIndex === -1) continue;
    
    const key = trimmed.slice(0, colonIndex).trim();
    const value = trimmed.slice(colonIndex + 1).trim();

    if (indent === 0) {
      // Top-level key
      if (value === "" || value === undefined) {
        // Section header
        currentSection = key;
        result[key] = {};
      } else {
        result[key] = value;
        currentSection = null;
      }
    } else if (currentSection) {
      // Nested under current section
      (result[currentSection] as Record<string, unknown>)[key] = value;
    }
  }

  return result;
}

/**
 * Load configuration from file
 */
async function loadConfigFile(): Promise<AgenatorConfig> {
  try {
    const configPath = path.join(CONFIG_DIR, CONFIG_FILE);
    const content = await fs.readFile(configPath, "utf-8");
    const parsed = parseYaml(content);
    
    return {
      linear: parsed.linear as AgenatorConfig["linear"],
      agent: parsed.agent as AgenatorConfig["agent"],
      workspace: parsed.workspace as AgenatorConfig["workspace"],
    };
  } catch {
    // Config file doesn't exist or couldn't be read
    return {};
  }
}

/**
 * Get Linear API key
 * Priority: env var > config file
 */
export async function getLinearApiKey(): Promise<string | undefined> {
  // Check environment variable first
  const envKey = process.env.AGENATOR_LINEAR_API_KEY;
  if (envKey) return envKey;

  // Fall back to config file
  const config = await loadConfigFile();
  return config.linear?.apiKey;
}

/**
 * Get Linear default team
 */
export async function getLinearDefaultTeam(): Promise<string | undefined> {
  const config = await loadConfigFile();
  return config.linear?.defaultTeam;
}

/**
 * Load full configuration
 */
export async function loadConfig(): Promise<AgenatorConfig> {
  const config = await loadConfigFile();
  
  // Override with environment variables
  if (process.env.AGENATOR_LINEAR_API_KEY) {
    config.linear = config.linear || {};
    config.linear.apiKey = process.env.AGENATOR_LINEAR_API_KEY;
  }

  return config;
}

/**
 * Ensure config directory exists
 */
export async function ensureConfigDir(): Promise<void> {
  await fs.mkdir(CONFIG_DIR, { recursive: true });
}
