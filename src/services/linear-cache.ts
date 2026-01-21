/**
 * Linear Ticket Cache Layer
 * 
 * Simple file-based cache with 5-minute TTL for offline support
 * and reduced API calls.
 */

import * as fs from "fs/promises";
import * as path from "path";
import type { LinearTicket } from "../types.js";

interface CacheEntry<T> {
  data: T;
  timestamp: number;
}

const CACHE_DIR = `${process.env.HOME}/.agenator/cache`;
const CACHE_FILE = "tickets.json";
const DEFAULT_TTL = 5 * 60 * 1000; // 5 minutes

interface TicketCache {
  tickets: Record<string, CacheEntry<LinearTicket>>;
  searchResults: Record<string, CacheEntry<string[]>>; // query -> ticket identifiers
}

async function loadCache(): Promise<TicketCache> {
  try {
    const data = await fs.readFile(path.join(CACHE_DIR, CACHE_FILE), "utf-8");
    return JSON.parse(data);
  } catch {
    return { tickets: {}, searchResults: {} };
  }
}

async function saveCache(cache: TicketCache): Promise<void> {
  await fs.mkdir(CACHE_DIR, { recursive: true });
  await fs.writeFile(
    path.join(CACHE_DIR, CACHE_FILE),
    JSON.stringify(cache, null, 2)
  );
}

/**
 * Get a single ticket from cache
 */
export async function getCachedTicket(identifier: string): Promise<LinearTicket | null> {
  const cache = await loadCache();
  const entry = cache.tickets[identifier];
  
  if (entry && Date.now() - entry.timestamp < DEFAULT_TTL) {
    return entry.data;
  }
  return null;
}

/**
 * Cache a single ticket
 */
export async function cacheTicket(ticket: LinearTicket): Promise<void> {
  const cache = await loadCache();
  cache.tickets[ticket.identifier] = {
    data: ticket,
    timestamp: Date.now(),
  };
  await saveCache(cache);
}

/**
 * Cache multiple tickets and search results
 */
export async function cacheSearchResults(query: string, tickets: LinearTicket[]): Promise<void> {
  const cache = await loadCache();
  
  // Cache individual tickets
  for (const ticket of tickets) {
    cache.tickets[ticket.identifier] = {
      data: ticket,
      timestamp: Date.now(),
    };
  }
  
  // Cache search results as list of identifiers
  cache.searchResults[query.toLowerCase()] = {
    data: tickets.map(t => t.identifier),
    timestamp: Date.now(),
  };
  
  await saveCache(cache);
}

/**
 * Get cached search results
 */
export async function getCachedSearchResults(query: string): Promise<LinearTicket[] | null> {
  const cache = await loadCache();
  const entry = cache.searchResults[query.toLowerCase()];
  
  if (entry && Date.now() - entry.timestamp < DEFAULT_TTL) {
    const tickets: LinearTicket[] = [];
    for (const id of entry.data) {
      const ticketEntry = cache.tickets[id];
      if (ticketEntry && Date.now() - ticketEntry.timestamp < DEFAULT_TTL) {
        tickets.push(ticketEntry.data);
      }
    }
    // Only return if we have all the tickets
    return tickets.length === entry.data.length ? tickets : null;
  }
  return null;
}

/**
 * Invalidate a specific ticket (called after updates)
 */
export async function invalidateCachedTicket(identifier: string): Promise<void> {
  const cache = await loadCache();
  
  // Remove the ticket
  delete cache.tickets[identifier];
  
  // Remove from any search results that contain it
  for (const [query, entry] of Object.entries(cache.searchResults)) {
    if (entry.data.includes(identifier)) {
      delete cache.searchResults[query];
    }
  }
  
  await saveCache(cache);
}

/**
 * Clear entire cache
 */
export async function clearCache(): Promise<void> {
  await saveCache({ tickets: {}, searchResults: {} });
}
