/**
 * useLinear Hook
 * 
 * React hook wrapping Linear service with React lifecycle.
 * Integrates with cache layer for better performance.
 */

import { useState, useEffect, useCallback } from "react";
import type { LinearTicket } from "../types.js";
import * as linear from "../services/linear.js";
import * as cache from "../services/linear-cache.js";

interface CreateTicketOpts {
  teamId: string;
  title: string;
  description?: string;
}

interface UseLinearResult {
  isInitialized: boolean;
  isLoading: boolean;
  error: string | null;
  searchTickets: (query: string) => Promise<LinearTicket[]>;
  getTicket: (identifier: string) => Promise<LinearTicket | null>;
  updateTicketStatus: (identifier: string, stateId: string) => Promise<boolean>;
  createTicket: (opts: CreateTicketOpts) => Promise<LinearTicket>;
}

export function useLinear(apiKey?: string): UseLinearResult {
  const [isInitialized, setIsInitialized] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Initialize Linear client when API key is provided
  useEffect(() => {
    if (apiKey) {
      linear.initializeLinear(apiKey);
      setIsInitialized(true);
      setError(null);
    } else {
      setIsInitialized(false);
    }
  }, [apiKey]);

  const searchTickets = useCallback(async (query: string): Promise<LinearTicket[]> => {
    if (!isInitialized) {
      throw new Error("Linear not initialized");
    }

    setIsLoading(true);
    setError(null);

    try {
      // Try cache first
      const cached = await cache.getCachedSearchResults(query);
      if (cached) {
        setIsLoading(false);
        return cached;
      }

      // Fetch from API
      const tickets = await linear.searchTickets(query);
      await cache.cacheSearchResults(query, tickets);
      return tickets;
    } catch (err) {
      const message = err instanceof Error ? err.message : "Search failed";
      setError(message);
      throw err;
    } finally {
      setIsLoading(false);
    }
  }, [isInitialized]);

  const getTicket = useCallback(async (identifier: string): Promise<LinearTicket | null> => {
    if (!isInitialized) {
      throw new Error("Linear not initialized");
    }

    setIsLoading(true);
    setError(null);

    try {
      // Try cache first
      const cached = await cache.getCachedTicket(identifier);
      if (cached) {
        setIsLoading(false);
        return cached;
      }

      // Fetch from API
      const ticket = await linear.getTicket(identifier);
      if (ticket) {
        await cache.cacheTicket(ticket);
      }
      return ticket;
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to get ticket";
      setError(message);
      throw err;
    } finally {
      setIsLoading(false);
    }
  }, [isInitialized]);

  const updateTicketStatus = useCallback(async (
    identifier: string, 
    stateId: string
  ): Promise<boolean> => {
    if (!isInitialized) {
      throw new Error("Linear not initialized");
    }

    setIsLoading(true);
    setError(null);

    try {
      const success = await linear.updateTicketState(identifier, stateId);
      if (success) {
        // Invalidate cache for this ticket
        await cache.invalidateCachedTicket(identifier);
      }
      return success;
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to update ticket";
      setError(message);
      throw err;
    } finally {
      setIsLoading(false);
    }
  }, [isInitialized]);

  const createTicket = useCallback(async (opts: CreateTicketOpts): Promise<LinearTicket> => {
    if (!isInitialized) {
      throw new Error("Linear not initialized");
    }

    setIsLoading(true);
    setError(null);

    try {
      const ticket = await linear.createTicket(opts.teamId, opts.title, opts.description);
      await cache.cacheTicket(ticket);
      return ticket;
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to create ticket";
      setError(message);
      throw err;
    } finally {
      setIsLoading(false);
    }
  }, [isInitialized]);

  return {
    isInitialized,
    isLoading,
    error,
    searchTickets,
    getTicket,
    updateTicketStatus,
    createTicket,
  };
}
